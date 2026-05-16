use std::env;

fn main() {
    let mut args = env::args();
    let command_name = args.next().unwrap_or_else(|| "number-adder".to_string());
    let input_arguments: Vec<String> = args.collect();

    let numbers = match parse_numbers(&input_arguments) {
        Ok(numbers) => numbers,
        Err(error_message) => {
            eprintln!("{}", error_message);
            eprintln!("Usage: {} <num> [num ...]", command_name);
            std::process::exit(1);
        }
    };

    let sum: i32 = numbers.iter().sum();

    println!("The sum of {} is {}", format_number_list(&numbers), sum);
}

fn parse_numbers(input_arguments: &[String]) -> Result<Vec<i32>, String> {
    if input_arguments.is_empty() {
        return Err("at least one number is required".to_string());
    }

    input_arguments
        .iter()
        .enumerate()
        .map(|(argument_index, input_argument)| {
            input_argument.parse::<i32>().map_err(|_| {
                format!(
                    "argument {} ('{}') must be a valid integer",
                    argument_index + 1,
                    input_argument
                )
            })
        })
        .collect()
}

fn format_number_list(numbers: &[i32]) -> String {
    match numbers {
        [] => String::new(),
        [only_number] => only_number.to_string(),
        [first_number, second_number] => format!("{} and {}", first_number, second_number),
        _ => {
            let mut formatted_numbers: Vec<String> =
                numbers.iter().map(|number| number.to_string()).collect();
            let last_number = formatted_numbers
                .pop()
                .expect("number list is known to contain at least three values");

            format!("{}, and {}", formatted_numbers.join(", "), last_number)
        }
    }
}

#[cfg(test)]
mod tests {
    use super::{format_number_list, parse_numbers};

    #[test]
    fn parse_numbers_accepts_one_argument() {
        let input_arguments = vec!["5".to_string()];

        let numbers = parse_numbers(&input_arguments).expect("argument should parse");

        assert_eq!(numbers, vec![5]);
    }

    #[test]
    fn parse_numbers_accepts_many_arguments() {
        let input_arguments = vec!["1".to_string(), "2".to_string(), "3".to_string()];

        let numbers = parse_numbers(&input_arguments).expect("arguments should parse");

        assert_eq!(numbers, vec![1, 2, 3]);
    }

    #[test]
    fn parse_numbers_rejects_empty_arguments() {
        let input_arguments = Vec::new();

        let error_message =
            parse_numbers(&input_arguments).expect_err("empty arguments should fail");

        assert_eq!(error_message, "at least one number is required");
    }

    #[test]
    fn parse_numbers_rejects_invalid_integer() {
        let input_arguments = vec!["1".to_string(), "abc".to_string()];

        let error_message =
            parse_numbers(&input_arguments).expect_err("invalid integer should fail");

        assert_eq!(error_message, "argument 2 ('abc') must be a valid integer");
    }

    #[test]
    fn format_number_list_formats_one_number() {
        assert_eq!(format_number_list(&[5]), "5");
    }

    #[test]
    fn format_number_list_formats_two_numbers() {
        assert_eq!(format_number_list(&[1, 2]), "1 and 2");
    }

    #[test]
    fn format_number_list_formats_many_numbers() {
        assert_eq!(format_number_list(&[1, 2, 3]), "1, 2, and 3");
    }
}
