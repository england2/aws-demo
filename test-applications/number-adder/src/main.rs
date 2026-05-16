use std::{env, process::ExitCode};

fn main() -> ExitCode {
    let mut args = env::args();
    let program_name = args.next().unwrap_or_else(|| "number-adder".to_string());
    let values: Vec<String> = args.collect();

    if values.is_empty() {
        print_usage(&program_name);
        return ExitCode::FAILURE;
    }

    let numbers = match parse_numbers(&values) {
        Ok(numbers) => numbers,
        Err(message) => {
            eprintln!("{message}");
            print_usage(&program_name);
            return ExitCode::FAILURE;
        }
    };

    let sum: i32 = numbers.iter().sum();

    println!("The sum is {sum}");
    ExitCode::SUCCESS
}

fn parse_numbers(values: &[String]) -> Result<Vec<i32>, String> {
    values
        .iter()
        .enumerate()
        .map(|(index, value)| {
            value
                .parse()
                .map_err(|_| format!("argument {} must be a valid integer: {value}", index + 1))
        })
        .collect()
}

fn print_usage(program_name: &str) {
    eprintln!("Usage: {program_name} <num> [num ...]");
}

#[cfg(test)]
mod tests {
    use super::parse_numbers;

    #[test]
    fn parses_any_number_of_integers() {
        let values = vec!["1".to_string(), "2".to_string(), "3".to_string()];

        let numbers = parse_numbers(&values).expect("numbers should parse");

        assert_eq!(numbers, vec![1, 2, 3]);
    }

    #[test]
    fn reports_invalid_integer_argument() {
        let values = vec!["1".to_string(), "nope".to_string()];

        let error = parse_numbers(&values).expect_err("invalid integer should fail");

        assert_eq!(error, "argument 2 must be a valid integer: nope");
    }
}
