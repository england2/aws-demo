use std::env;

fn main() {
    let args: Vec<String> = env::args().collect();

    if args.len() != 3 {
        eprintln!("Usage: {} <num1> <num2>", args[0]);
        std::process::exit(1);
    }

    let x: i32 = args[1]
        .parse()
        .expect("arg 1 must be a valid integer");

    let y: i32 = args[2]
        .parse()
        .expect("arg 2 must be a valid integer");

    let sum = x + y;

    println!("The sum of {} and {} is {}", x, y, sum);
}
