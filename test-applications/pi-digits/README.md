# pi-digits

`pi-digits` prints pi to a requested number of digits after the decimal point.
It uses an integer spigot algorithm, so output is deterministic and is not
limited by floating-point precision.

```sh
go run . 100
go run . -digits 100
```

