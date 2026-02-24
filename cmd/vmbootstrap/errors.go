package main

type userError struct {
	msg  string
	hint string
}

func (e *userError) Error() string { return e.msg }
func (e *userError) Hint() string  { return e.hint }
