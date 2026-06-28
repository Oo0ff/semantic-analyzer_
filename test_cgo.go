package main
// #include <stdio.h>
// void hello() { printf("Hello from C via CGO!\n"); }
import "C"
func main() {
    C.hello()
}