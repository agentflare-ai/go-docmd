// Package example demonstrates documentation rendering for go-docmd tests.
//
// Features:
//   - **Alpha**: demonstrates bold formatting preservation.
//   - **Beta**: verifies list items stay intact.
package example

const (
	// Answer documents an exported constant.
	Answer = 42

	// hidden constant should be available with -u.
	internalConstant = 0
)

// Greeter produces greeting messages.
type Greeter struct {
	// Name is included to verify field documentation.
	Name string
}

// NewGreeter constructs a Greeter.
func NewGreeter(name string) *Greeter {
	return &Greeter{Name: name}
}

// Greet returns a friendly message.
func (g *Greeter) Greet() string {
	return "hello " + g.Name
}
