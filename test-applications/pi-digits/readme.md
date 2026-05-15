# Pi Digits

`pi-digits` prints pi with the requested number of digits after the decimal
point.

```sh
go run . 50
```

The implementation uses the Chudnovsky formula with binary splitting and
standard-library arbitrary precision integers.
