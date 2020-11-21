package cmd

import (
	"fmt"
	"log"
	"strings"

	"github.com/pkg/errors"
)

func logStackTrace(err error, debug bool) {
	if err != nil && debug {
		if e := traceableRootCause(err); e != nil {
			if s := fmt.Sprintf("%+v", e); s != err.Error() {
				log.Println("DEBUG:", strings.ReplaceAll(s, "\n", "\n  "))
			}
		}
	}
}

func traceableRootCause(err error) error {
	cause := errors.Unwrap(err)
	if cause != nil && cause != err {
		rootCause := traceableRootCause(cause)
		if hasMoreInfo(rootCause, cause) {
			return rootCause
		}
		if hasMoreInfo(cause, err) {
			return cause
		}
	}
	return err
}

func hasMoreInfo(err, than error) bool {
	s := than.Error()
	for _, l := range strings.Split(fmt.Sprintf("%+v", err), "\n") {
		if !strings.Contains(s, l) {
			return true
		}
	}
	return false
}
