package main

import (
	"fmt"
	"regexp"
)

type Person struct {
	Name    string
	Surname string
}

func Run() (err error) {
	/*
		defer func() {
			fmt.Println("defer 3")
			//if r := recover(); r != nil {
			//	err = fmt.Errorf("panic %v", r)
			//}
		}()
	*/

	var p *Person
	p.Surname = "kek"

	var i any = p
	_ = i.(int)

	vec := []int{1, 2, 3}
	vec[5]++

	i, _ = regexp.Compile("(?P<Year>\\d{4}") // MustCompile

	panic("test")

	return nil
}

func main() {
	if err := Run(); err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println("ok")
}
