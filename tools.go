//go:build tools

// Package tools tracks tool and future dependencies.
// This file is excluded from normal builds.
package tools

import (
	// Bubbles widget library — used for file lists, text inputs, viewports, etc.
	_ "charm.land/bubbles/v2/viewport"
)
