"""Bound local source requests without changing measured-exit HTTP clients."""
from queue import Empty, Queue
from threading import Thread
import http.client
import socket
import time
import urllib.error
import urllib.request

from .common import HttpsRedirect, ServiceError, ToolError, get_bytes, https_url


def _connect_source(address, timeout, source_address=None):
    # Share the connect budget across resolved addresses. A dead first address
    # must not consume every second while later addresses are never attempted.
    deadline = time.monotonic() + timeout
    addresses = socket.getaddrinfo(*address, 0, socket.SOCK_STREAM)
    for i, (family, kind, proto, _, sockaddr) in enumerate(addresses):
        remaining = deadline - time.monotonic()
        if remaining <= 0:
            break
        connection = socket.socket(family, kind, proto)
        try:
            connection.settimeout(remaining / (len(addresses)-i))
            if source_address:
                connection.bind(source_address)
            connection.connect(sockaddr)
            connection.settimeout(max(0.01, deadline-time.monotonic()))
            return connection
        except OSError:
            connection.close()
    raise TimeoutError('source_connect_timeout')


class _SourceHTTPSConnection(http.client.HTTPSConnection):
    def connect(self):
        self._create_connection = _connect_source
        # Standard TLS context, hostname/SNI and certificate verification stay
        # with HTTPSConnection. Only candidate-source address dialing differs.
        super().connect()


class _SourceHTTPSHandler(urllib.request.HTTPSHandler):
    def https_open(self, req):
        return self.do_open(_SourceHTTPSConnection, req, context=self._context)


def _get_source_bytes(url, timeout, limit):
    https_url(url)
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), HttpsRedirect(), _SourceHTTPSHandler())
    deadline = time.monotonic()+timeout
    for attempt in range(2):
        remaining = deadline-time.monotonic()
        if remaining <= 0:
            raise ServiceError('source_request_timeout')
        try:
            request = urllib.request.Request(url, headers={'User-Agent':'VPNGate-Toolbox/0.1'})
            with opener.open(request, timeout=remaining) as response:
                data = response.read(limit+1)
            if len(data)>limit:raise ServiceError('response_too_large')
            return data
        except urllib.error.HTTPError as exc:
            if exc.code >= 500 and attempt == 0:continue
            raise ServiceError('http_'+str(exc.code)) from None
        except (urllib.error.URLError, TimeoutError, OSError):
            if attempt == 0:continue
            raise ServiceError('https_unavailable') from None
    raise ServiceError('https_unavailable')


class SourceFetcher:
    def __init__(self, transport=get_bytes, budget=60):
        self.transport = transport
        self.deadline = time.monotonic() + budget

    @property
    def remaining(self):
        return max(0, self.deadline - time.monotonic())

    def fetch(self, url, timeout, limit=12_000_000):
        timeout = min(timeout, self.remaining)
        if timeout <= 0:
            raise ServiceError('source_request_timeout')
        result = Queue(maxsize=1)

        def request():
            try:
                kwargs = {'timeout': timeout}
                if self.transport is get_bytes:
                    value = _get_source_bytes(url, timeout, limit)
                else:
                    value = self.transport(url, **kwargs)
                if not isinstance(value, bytes) or len(value) > limit:
                    raise ServiceError('response_too_large')
                result.put((value, None))
            except ToolError as exc:
                result.put((None, exc))
            except Exception:
                result.put((None, ServiceError('source_request_failed')))

        # getaddrinfo and sequential address connects may outlive a socket's
        # timeout. The worker can only return bytes, never publish files. Late
        # results are ignored, and daemon workers cannot hold this CLI alive.
        Thread(target=request, daemon=True).start()
        try:
            data, error = result.get(timeout=timeout)
        except Empty:
            raise ServiceError('source_request_timeout') from None
        if error is not None:
            raise error
        return data
