package main

import "time"

type CountryEvidence struct {
	IP                string `json:"ip"`
	Country           string `json:"country"`
	Status            string `json:"status"`
	Provider          string `json:"provider"`
	QueriedAt         string `json:"queried_at"`
	ExpiresAt         string `json:"expires_at"`
	IdentityAssurance string `json:"identity_assurance"`
	Error             string `json:"error,omitempty"`
}

func evidenceFresh(expiry string) bool {
	t, err := time.Parse(time.RFC3339Nano, expiry)
	return err == nil && t.After(time.Now())
}
func verifiedProfileCountry(p EgressProfile) string {
	code := stringValue(p.Country)
	if flagOf(code) == "" || code == "ZZ" || code == "XX" {
		return ""
	}
	if p.IntelligenceStatus == "passed" && evidenceFresh(p.ExpiresAt) {
		return code
	}
	c := p.CountryEvidence
	if c != nil && c.Status == "passed" && c.IP == p.IP && c.Country == code && c.Provider != "" && c.QueriedAt != "" && c.IdentityAssurance == "normal_https_certificate_validation" && evidenceFresh(c.ExpiresAt) {
		return code
	}
	return ""
}
