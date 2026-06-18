pub enum CalcError {
    DivideByZero,
}

pub struct Calculator {
    history: Vec<String>,
}

impl Calculator {
    pub fn new() -> Calculator {
        Calculator {
            history: Vec::new(),
        }
    }

    fn log_operation(&mut self, operation: String) {
        self.history.push(operation);
    }

    pub fn add(&mut self, a: f64, b: f64) -> f64 {
        let result = a + b;
        self.log_operation(format!("{} + {} = {}", a, b, result));
        result
    }

    pub fn subtract(&mut self, a: f64, b: f64) -> f64 {
        let result = a - b;
        self.log_operation(format!("{} - {} = {}", a, b, result));
        result
    }

    pub fn multiply(&mut self, a: f64, b: f64) -> f64 {
        let result = a * b;
        self.log_operation(format!("{} * {} = {}", a, b, result));
        result
    }

    pub fn divide(&mut self, a: f64, b: f64) -> Result<f64, CalcError> {
        if b == 0.0 {
            self.log_operation(format!("{} / {} = DivideByZero", a, b));
            Err(CalcError::DivideByZero)
        } else {
            let result = a / b;
            self.log_operation(format!("{} / {} = {}", a, b, result));
            Ok(result)
        }
    }

    pub fn history(&self) -> &Vec<String> {
        &self.history
    }
}
