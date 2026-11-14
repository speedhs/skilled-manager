package core_test

import (
	"fmt"

	"github.com/speedhs/skilled-manager/pkg/core"
)

func ExampleValidateSkillName() {
	fmt.Println(core.ValidateSkillName("code-reviewer"))
	fmt.Println(core.ValidateSkillName("..") != nil)
	fmt.Println(core.ValidateSkillName("CON") != nil)
	// Output:
	// <nil>
	// true
	// true
}

func ExampleExpandPath() {
	path, err := core.ExpandPath("${SKILLS_EXAMPLE_UNSET:-skills}/demo")
	fmt.Println(err)
	fmt.Println(path)
	_, err = core.ExpandPath("$SKILLS_EXAMPLE_UNSET")
	fmt.Println(err != nil)
	// Output:
	// <nil>
	// skills/demo
	// true
}
