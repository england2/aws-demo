use std::env;

fn main() {
    let args: Vec<String> = env::args().collect();
    let program_name = &args[0];
    let numbers = &args[1..];

    if numbers.is_empty() {
        eprintln!("Usage: {} <num> [num ...]", program_name);
        std::process::exit(1);
    }

    let mut sum = 0;
    for (index, arg) in numbers.iter().enumerate() {
        let value: i32 = arg
            .parse()
            .unwrap_or_else(|_| panic!("arg {} must be a valid integer", index + 1));
        sum += value;
    }

    let number_list = if numbers.len() == 2 {
        format!("{} and {}", numbers[0], numbers[1])
    } else {
        numbers.join(" + ")
    };

    println!("The sum of {} is {}", number_list, sum);
}
