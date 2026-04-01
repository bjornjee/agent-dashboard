/**
 * Simple calculator module for testing diff display.
 */

function add(a, b) {
  return a + b;
}

function subtract(a, b) {
  return a - b;
}

function multiply(a, b) {
  return a * b;
}

function divide(a, b) {
  if (b === 0) {
    throw new Error("Cannot divide by zero");
  }
  return a / b;
}

class Calculator {
  constructor() {
    this.history = [];
  }

  calculate(operation, a, b) {
    const ops = { add, subtract, multiply, divide };
    if (!(operation in ops)) {
      throw new Error(`Unknown operation: ${operation}`);
    }

    const result = ops[operation](a, b);
    this.history.push(`${operation}(${a}, ${b}) = ${result}`);
    return result;
  }

  getHistory() {
    return [...this.history];
  }

  clearHistory() {
    this.history = [];
  }
}

module.exports = { add, subtract, multiply, divide, Calculator };
