// +build mandate_fips

package main

import (
	_ "crypto/tls/fipsonly"
)

func init() {
	fipsMode = true
}
