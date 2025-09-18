// Package geo provides IP geolocation services for DNS servers.
package geo

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/oschwald/geoip2-golang"
)

// GeoService provides IP geolocation services
type GeoService struct {
	db *geoip2.Reader
}

// NewGeoService creates a new geo service with embedded GeoIP data
func NewGeoService() (*GeoService, error) {
	// Try to load the GeoIP database from common locations
	dbPaths := []string{
		"res/Country.mmdb",
		"../res/Country.mmdb",
		"frontend/res/Country.mmdb",
		"./Country.mmdb",
	}

	var db *geoip2.Reader
	var err error

	for _, path := range dbPaths {
		db, err = geoip2.Open(path)
		if err == nil {
			break
		}
	}

	if err != nil {
		return nil, fmt.Errorf("GeoIP service not available - database not found: %v", err)
	}

	return &GeoService{db: db}, nil
}

// CheckGeo analyzes a DNS server address and returns its IP and country code
func (g *GeoService) CheckGeo(server string, preferIPv4 bool) (string, string, error) {
	if g.db == nil {
		return "0.0.0.0", "UNKNOWN", fmt.Errorf("GeoIP database not available")
	}

	server = strings.TrimSpace(server)
	server = strings.TrimSuffix(server, "/")
	if server == "" {
		return "0.0.0.0", "PRIVATE", fmt.Errorf("empty server address")
	}

	var ip net.IP
	if strings.Contains(server, "://") {
		// URL format
		server = strings.TrimPrefix(server, "https://")
		server = strings.TrimPrefix(server, "tls://")
		server = strings.TrimPrefix(server, "quic://")
		server = strings.TrimPrefix(server, "http://")

		if strings.Contains(server, "/") {
			// Contains path
			parts := strings.SplitN(server, "/", 2)
			server = parts[0]
		}
		if strings.Contains(server, "[") && strings.Contains(server, "]") {
			// IPv6 URL
			server = strings.SplitN(server, "]", 2)[0]
			server = strings.TrimPrefix(server, "[")
		} else if strings.Contains(server, ":") {
			// URL with port
			parts := strings.SplitN(server, ":", 2)
			server = parts[0]
		}

		// Resolve to IP
		ips, err := net.LookupIP(server)
		if err != nil || len(ips) == 0 {
			return "0.0.0.0", "PRIVATE", fmt.Errorf("unable to resolve IP address")
		}

		if len(ips) == 1 {
			ip = ips[0]
		} else if preferIPv4 {
			for _, _ip := range ips {
				if _ip.To4() != nil {
					ip = _ip
					break
				}
			}
			if ip == nil {
				ip = ips[0]
			}
		} else {
			ip = ips[0]
		}
	} else {
		// IP address or hostname
		parts := strings.SplitN(server, ":", 2)
		if len(parts) > 1 {
			if port, err := strconv.Atoi(parts[1]); err == nil && port > 0 && port < 65536 {
				server = parts[0]
			}
		}

		ips, err := net.LookupIP(server)
		if err != nil || len(ips) == 0 {
			return "0.0.0.0", "PRIVATE", fmt.Errorf("local resolver cannot resolve host IP address")
		}
		ip = ips[0]
	}

	if ip.IsPrivate() || ip.IsUnspecified() {
		return ip.String(), "PRIVATE", nil
	}

	geoCode, err := g.checkIPGeo(ip)
	return ip.String(), geoCode, err
}

// checkIPGeo queries the GeoIP database for country information
func (g *GeoService) checkIPGeo(ip net.IP) (string, error) {
	if g.db == nil {
		return "UNKNOWN", fmt.Errorf("GeoIP database not available")
	}

	record, err := g.db.Country(ip)
	if err != nil {
		return "CDN", err
	}
	if record.Country.IsoCode == "" {
		return "CDN", nil
	}
	return record.Country.IsoCode, nil
}

// Close closes the GeoIP database
func (g *GeoService) Close() error {
	if g.db != nil {
		return g.db.Close()
	}
	return nil
}
