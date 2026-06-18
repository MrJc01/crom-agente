package calculator

import (
	"fmt"
	"math"
	"strconv"
)

// Divide performs division of two float64 numbers.
// It returns an error if the divisor is zero.
func Divide(a, b float64) (float64, error) {
	if b == 0.0 {
		return 0.0, fmt.Errorf("divisão por zero não é permitida")
	}
	return a / b
}

// Factorial calculates the factorial of a non-negative integer.
func Factorial(n int) int {
	if n < 0 {
		return 0 // ou retornar um erro, dependendo dos requisitos
	}
	if n <= 1 {
		return 1
	}
	result := 1
	for i := 2; i <= n; i++ {
		result *= i
	}
	return result
}

// Sqrt calculates the square root of a non-negative float64 number.
// It returns an error if the input is negative.
func Sqrt(x float64) (float64, error) {
	if x < 0 {
		return 0, fmt.Errorf("não é possível calcular raiz de número negativo: %f", x)
	}
	return math.Sqrt(x), nil
}

// Power calculates base raised to the power of exp.
func Power(base, exp int) int {
	result := 1
	for i := 0; i < exp; i++ {
		result *= base
	}
	return result
}

// ParseNumber converts a string to an integer.
// It returns an error if the string cannot be converted.
func ParseNumber(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("falha ao converter string para int: %w", err)
	}
	return n, nil
}
