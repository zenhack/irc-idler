package irc

import (
	"fmt"
)

type NotEnoughParams struct {
	Wanted, Got int
}

func (e NotEnoughParams) Error() string {
	return fmt.Sprintf(
		"Not enough parameters; wanted %d but got %d.",
		e.Wanted, e.Got)
}
