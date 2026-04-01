"""Simple calculator module for testing diff display."""


def add(a: float, b: float) -> float:
    """Add two numbers."""
    return a + b


def subtract(a: float, b: float) -> float:
    """Subtract b from a."""
    return a - b


def multiply(a: float, b: float) -> float:
    """Multiply two numbers."""
    return a * b


def divide(a: float, b: float) -> float:
    """Divide a by b, returning a float result."""
    if b == 0:
        raise ZeroDivisionError("Cannot divide by zero")
    return a / b


def power(a: float, b: float) -> float:
    """Raise a to the power of b."""
    return a ** b


class Calculator:
    """A simple calculator with history."""

    def __init__(self):
        self.history: list[str] = []

    def calculate(self, operation: str, a: float, b: float) -> float:
        ops = {
            "add": add,
            "subtract": subtract,
            "multiply": multiply,
            "divide": divide,
            "power": power,
        }
        if operation not in ops:
            raise ValueError(f"Unknown operation: {operation}")

        result = ops[operation](a, b)
        self.history.append(f"{operation}({a}, {b}) = {result}")
        return result

    def get_history(self) -> list[str]:
        return self.history.copy()

    def clear_history(self) -> None:
        self.history.clear()
