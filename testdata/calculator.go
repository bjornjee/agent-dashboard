package testdata

import (
	"errors"
	"fmt"
)

// Calculator performs basic arithmetic with history tracking.
type Calculator struct {
	history []string
}

// NewCalculator creates a new Calculator instance.
func NewCalculator() *Calculator {
	return &Calculator{
		history: make([]string, 0),
	}
}

// Add returns the sum of a and b.
func (c *Calculator) Add(a, b float64) float64 {
	result := a + b
	c.history = append(c.history, fmt.Sprintf("add(%g, %g) = %g", a, b, result))
	return result
}

// Subtract returns a minus b.
func (c *Calculator) Subtract(a, b float64) float64 {
	result := a - b
	c.history = append(c.history, fmt.Sprintf("subtract(%g, %g) = %g", a, b, result))
	return result
}

// Multiply returns the product of a and b.
func (c *Calculator) Multiply(a, b float64) float64 {
	result := a * b
	c.history = append(c.history, fmt.Sprintf("multiply(%g, %g) = %g", a, b, result))
	return result
}

// Divide returns a divided by b. Returns an error if b is zero.
func (c *Calculator) Divide(a, b float64) (float64, error) {
	if b == 0 {
		return 0, errors.New("cannot divide by zero")
	}
	result := a / b
	c.history = append(c.history, fmt.Sprintf("divide(%g, %g) = %g", a, b, result))
	return result, nil
}

// Power returns a raised to the power of b.
func (c *Calculator) Power(a, b float64) float64 {
	result := 1.0
	for i := 0; i < int(b); i++ {
		result *= a
	}
	c.history = append(c.history, fmt.Sprintf("power(%g, %g) = %g", a, b, result))
	return result
}

// History returns a copy of the calculation history.
func (c *Calculator) History() []string {
	out := make([]string, len(c.history))
	copy(out, c.history)
	return out
}

// ClearHistory removes all history entries.
func (c *Calculator) ClearHistory() {
	c.history = c.history[:0]
}

// LastResult returns the most recent history entry, or empty string if none.
func (c *Calculator) LastResult() string {
	if len(c.history) == 0 {
		return ""
	}
	return c.history[len(c.history)-1]
}
