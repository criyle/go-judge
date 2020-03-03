// Package diff provides function to compare contents from reader and
// returns error information if they are different.
//
// The package will ignore white spaces at the end of line and end of file
package diff

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"unicode"
)

// Compare compares actual with expected.
// if they are the same except space at line / file ending,
// no error is returned
// otherwise, if error occurred / not same, error will be
// returned
func Compare(expected, actual io.Reader) error {
	expScan := bufio.NewScanner(expected)
	actScan := bufio.NewScanner(actual)

	for line := 1; ; line++ {
		exp, hasExp := scanTrimRight(expScan)
		act, hasAct := scanTrimRight(actScan)

		// EOF at the same time
		if !hasExp && !hasAct {
			return nil
		}
		// they are not equal
		if exp != act {
			return newErr(line, exp, act)
		}
		// they are all exists and equal
		if hasExp && hasAct {
			continue
		}
		// verify all empty line lefts
		if err := verifyEOFSpace("actual", actScan); err != nil {
			return err
		}
		if err := verifyEOFSpace("expected", expScan); err != nil {
			return err
		}
		// at this point, they should all be same
		return nil
	}
}

func newErr(line int, exp, act string) error {
	return fmt.Errorf("At line %d,\nexpected: %v\nactual: %v", line, exp, act)
}

func scanTrimRight(sc *bufio.Scanner) (string, bool) {
	if sc.Scan() {
		return trimRight(sc), true
	}
	return "", false
}

func verifyEOFSpace(name string, sc *bufio.Scanner) error {
	for sc.Scan() {
		if v := trimRight(sc); v != "" {
			return fmt.Errorf("%v have more content: %v", name, v)
		}
	}
	return nil
}

func trimRight(sc *bufio.Scanner) string {
	return string(bytes.TrimRightFunc(sc.Bytes(), unicode.IsSpace))
}
