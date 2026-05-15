use std::env;

fn main() {
    let args: Vec<String> = env::args().collect();
    let program_name = &args[0];

    if args.len() < 2 {
        eprintln!("Usage: {} <num> [num ...]", program_name);
        std::process::exit(1);
    }

    let mut sum = 0;

    for (index, arg) in args.iter().skip(1).enumerate() {
        let number: i32 = match arg.parse() {
            Ok(number) => number,
            Err(_) => {
                eprintln!("arg {} must be a valid integer", index + 1);
                std::process::exit(1);
            }
        };

        sum += number;
    }

    println!("The sum is {}", sum);
}
