# apicompat

This program reads a Go source directory and generates a file that uses the entire API. This ensures that if the API changes at any time, the file will fail to compile.

## Interfaces

It generates a struct that implements every interface method and asserts the struct implements the interface.

## Structs

It generates an interface with all of the methods attached to the struct and attempts to use the struct on that interface. It will also attempt to instantiate the struct with the zero value for every exported attribute.

## Variables and Constants

It will generate a variable name with the same name and assign the variable to the new variable.

TODO: Should it verify that constants don't change for things like enums? Changing enums may be considered a breaking change, but not all constants are like that.
