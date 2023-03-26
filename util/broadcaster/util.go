package broadcaster

import (
	"fmt"
)

func makeError(errors map[string]error) error {
	if len(errors) == 0 {
		return nil
	} else {
		errStr := ""
		for key, v := range errors {
			errStr += fmt.Sprintf("%s(%s).", key, v.Error())
		}
		return fmt.Errorf("%s", errStr)
	}
}
