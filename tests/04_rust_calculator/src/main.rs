/// Erros da calculadora
#[derive(Debug)]
pub enum CalcError {
    DivisionByZero,
    InvalidOperation(String),
}

impl std::fmt::Display for CalcError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            CalcError::DivisionByZero => write!(f, "Erro: divisão por zero"),
            CalcError::InvalidOperation(op) => write!(f, "Erro: operação inválida '{}'", op),
        }
    }
}

/// Calculadora com histórico de operações
pub struct Calculator {
    history: Vec<String>,
}

impl Calculator {
    /// Cria uma nova instância da calculadora
    pub fn new() -> Self {
        Calculator {
            history: Vec::new(),
        }
    }

    /// Soma dois números
    pub fn add(&mut self, a: f64, b: f64) -> f64 {
        let result = a + b;
        self.history.push(format!("{} + {} = {}", a, b, result));
        result
    }

    /// Subtrai b de a
    pub fn subtract(&mut self, a: f64, b: f64) -> f64 {
        let result = a - b;
        self.history.push(format!("{} - {} = {}", a, b, result));
        result
    }

    // TODO: Implementar multiply
    // TODO: Implementar divide (com tratamento de divisão por zero)
    // TODO: Implementar modulo
    // TODO: Implementar history()
}

fn main() {
    println!("Calculadora Rust — Digite uma expressão (ex: 2 + 3) ou 'quit' para sair");
    // TODO: Implementar loop de leitura interativa
}
