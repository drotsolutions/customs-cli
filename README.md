# Customs CLI

## What is this?

Customs CLI is a tool used for testing of [Drot Solutions'](https://drotsolutions.com/api/v1/docs/index.html) APIs.

## Installation

The only prerequisites for the installation is to have installed Go programming language.
On a Mac this can be done with `brew install go`, otherwise please follow [Go installation instructions](https://go.dev/doc/install).

Once you have Go installed, and you clone this repository, simply run `make` command.
This will build the binary named `customs` located in the current directory.

If you want to have the binary globally available, consider moving it to one of the directories that are in your PATH.

*Note: the examples below assume the binary is globally available, otherwise just replace the `customs` in the examples with `./customs`.*

## Usage

The main purpose of this program is to take data from a local Excel file, upload it to the customs service that will generate customs codes,
and write the generated customs codes to a local file.

The command expects the Excel file to have specific columns. Please check `examples/sample.xlsx` file for details.
The output file will have the same content as the input file, except it will contain additional columns with the generated customs codes.

Usage example:
```
customs --api-key "yourApiKey" input-file.xlsx
```

For more details please run:
```
customs --help
```
