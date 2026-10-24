"""Shared source-refresh gate for the UI, manager and systemd fallback."""
from datetime import datetime, timezone, timedelta
import json
from pathlib import Path

from .common import ToolError, write_json

DEFAULTS = {'refresh_enabled': True, 'refresh_minutes': 60,
            'check_enabled': True, 'check_minutes': 60}


def load_automation(directory):
    p = Path(directory)/'automation.json'
    values = dict(DEFAULTS)
    if p.exists():
        raw = json.loads(p.read_text())
        if not isinstance(raw, dict): raise ToolError('invalid_schedule_configuration')
        values.update(raw)
    for task in ('refresh', 'check'):
        if type(values[task+'_enabled']) is not bool or type(values[task+'_minutes']) is not int or not 30 <= values[task+'_minutes'] <= 525600:
            raise ToolError('invalid_schedule_configuration')
    return values


def parse_time(value):
    try:
        t = datetime.fromisoformat(value.replace('Z', '+00:00'))
        return t if t.tzinfo is not None else None
    except (AttributeError, ValueError, TypeError): return None


def source_reference(directory, output):
    p = Path(directory)/'refresh-state.json'
    if p.exists():
        value = json.loads(p.read_text())
        if not isinstance(value, dict): raise ToolError('invalid_schedule_state')
        ref = parse_time(value.get('finished_at')) or parse_time(value.get('started_at'))
        if value.get('status') not in ('running','passed','failed') or ref is None:
            raise ToolError('invalid_schedule_state')
        return ref
    p = Path(output)/'node_pool.json'
    if p.exists():
        try: return parse_time(json.loads(p.read_text()).get('generated_at'))
        except (ValueError, AttributeError): return None
    return None


def scheduled_pool(directory, output, scheduled, build, now=None):
    # Caller holds the Pool Builder lock. All contenders recheck the completion
    # time here; a second timer must never produce a second upstream request.
    directory, output = Path(directory), Path(output)
    config = load_automation(directory)
    clock = now or (lambda: datetime.now(timezone.utc))
    started = clock()
    if scheduled and not config['refresh_enabled']: return None
    previous = source_reference(directory, output) if scheduled else None
    if scheduled and (not config['refresh_enabled'] or
                      (previous is not None and started < previous+timedelta(minutes=config['refresh_minutes']))):
        return None
    state = {'status':'running', 'started_at':started.isoformat(), 'finished_at':None, 'nodes':0}
    write_json(directory/'refresh-state.json', state, private=True)
    try:
        pool = build()
        state.update(status='passed', nodes=len(pool['nodes']))
        return pool
    except Exception:
        state['status'] = 'failed'
        raise
    finally:
        state['finished_at'] = clock().isoformat()
        write_json(directory/'refresh-state.json', state, private=True)
