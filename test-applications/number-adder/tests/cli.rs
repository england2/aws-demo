use std::process::Command;

#[test]
fn adds_one_argument() {
    let output = Command::new(env!("CARGO_BIN_EXE_number-adder"))
        .arg("7")
        .output()
        .expect("number-adder should run");

    assert!(output.status.success());
    assert_eq!(
        String::from_utf8_lossy(&output.stdout),
        "The sum of 7 is 7\n"
    );
}

#[test]
fn preserves_two_argument_output() {
    let output = Command::new(env!("CARGO_BIN_EXE_number-adder"))
        .args(["1", "2"])
        .output()
        .expect("number-adder should run");

    assert!(output.status.success());
    assert_eq!(
        String::from_utf8_lossy(&output.stdout),
        "The sum of 1 and 2 is 3\n"
    );
}

#[test]
fn adds_a_variable_number_of_arguments() {
    let output = Command::new(env!("CARGO_BIN_EXE_number-adder"))
        .args(["1", "2", "3", "4"])
        .output()
        .expect("number-adder should run");

    assert!(output.status.success());
    assert_eq!(
        String::from_utf8_lossy(&output.stdout),
        "The sum of 1 + 2 + 3 + 4 is 10\n"
    );
}

#[test]
fn requires_at_least_one_argument() {
    let output = Command::new(env!("CARGO_BIN_EXE_number-adder"))
        .output()
        .expect("number-adder should run");

    assert!(!output.status.success());
    assert!(String::from_utf8_lossy(&output.stderr).contains("Usage:"));
}
