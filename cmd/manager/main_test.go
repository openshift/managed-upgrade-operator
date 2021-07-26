// +build testrunmain

package main

import (
  "net/http"
  _ "net/http/pprof"
  "testing"
)

func TestRunMain(t *testing.T) {
  go func() {
    http.ListenAndServe(":6060", nil)
  }()

  main()
}
