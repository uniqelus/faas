package harness

import (
	"bytes"
	"fmt"
)

func AssertContains(body []byte, expected string) error {
	if !bytes.Contains(body, []byte(expected)) {
		return fmt.Errorf("response body does not contain %q", expected)
	}
	return nil
}
