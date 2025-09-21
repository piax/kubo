package config

// HRNS contains the configuration for the Human-Readable Name System
type HRNS struct {
	// RepublishPeriod is the interval at which HRNS records are republished
	// Default: "1h"
	RepublishPeriod string `json:",omitempty"`
}
