package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
)

const (
	defaultURL    = "https://drotsolutions.com"
	defaultOutput = "result.xlsx"
)

const (
	customsTerritoryEU = "eu"
	customsTerritoryNO = "no"
)

var (
	allowedCustomsTerritories = []string{customsTerritoryEU, customsTerritoryNO}
)

var (
	ErrFailed       = fmt.Errorf("failed")
	ErrNotProcessed = fmt.Errorf("not processed")
)

var (
	help       bool
	apiKey     string
	url        string
	outputPath string
	timeout    int
)

func init() {
	flag.BoolVar(&help, "help", false, "")
	flag.StringVar(&apiKey, "api-key", "", "")
	flag.StringVar(&url, "url", defaultURL, "")
	flag.StringVar(&outputPath, "output", defaultOutput, "")
	flag.IntVar(&timeout, "timeout", 120, "")
}

func main() {
	flag.Parse()
	if help {
		fmt.Printf(`	Import items from an excel file and generate customs codes. The generated customs codes will be written to the provided output file (default %q).

	Options:
		--api-key	API key used for the authentication and authorization
		--url		URL of the server (default %q)
		--output	write output to the file (default %q).
		--timeout	how many seconds to wait on processing (default %d)
		--help		display this help and exit

	Example:
		customs --api-key "yourApiKey" input-file.xlsx

`, defaultOutput, defaultURL, defaultOutput, timeout)

		os.Exit(0)
	}

	if apiKey == "" {
		log.Fatalln("missing api-key flag")
	}
	if url == "" {
		log.Fatalln("missing url flag")
	}

	filePath := flag.Arg(0)
	if filePath == "" {
		log.Fatalln("please provide the excel file path as the command argument")
	}
	file, err := excelize.OpenFile(filePath)
	if err != nil {
		log.Fatalln(err)
	}
	defer func() {
		// Close the spreadsheet.
		if err = file.Close(); err != nil {
			log.Fatalln(err)
		}
	}()

	rows, err := file.GetRows("Sheet1")
	if err != nil {
		log.Fatalln(err)
	}

	if len(rows) < 2 {
		log.Fatalln("provided file is empty or it doesn't have the headings row")
	}

	headings := rows[0]
	iID, err := getMandatoryColumnIndex(headings, "id")
	if err != nil {
		log.Fatalln(err)
	}
	iName, err := getMandatoryColumnIndex(headings, "name")
	if err != nil {
		log.Fatalln(err)
	}
	iDescription, err := getMandatoryColumnIndex(headings, "description")
	if err != nil {
		log.Fatalln(err)
	}
	iCustomsTerritories, err := getMandatoryColumnIndex(headings, "customs territories")
	if err != nil {
		log.Fatalln(err)
	}

	iCategory := getColumnIndex(headings, "category")
	iSubcategory := getColumnIndex(headings, "subcategory")
	iCountryOfOrigin := getColumnIndex(headings, "country of origin")
	iGrossMass := getColumnIndex(headings, "gross mass")
	iNetMass := getColumnIndex(headings, "net mass")
	iWeightUnit := getColumnIndex(headings, "weight unit")
	iModel := getColumnIndex(headings, "model")

	// Append result columns.
	iResultEU := len(headings)
	headings = append(headings, "result EU")
	iResultNO := len(headings)
	headings = append(headings, "result NO")

	// Write headings to the output, because we have modified them by appending the result columns.
	err = file.SetSheetRow("Sheet1", "A1", &headings)
	if err != nil {
		log.Fatalln(err)
	}

	imp := ImportRequest{
		ImportItems: make([]ImportItemRequest, len(rows[1:])),
	}

	for i, row := range rows[1:] {
		id := getString(row, &iID)
		name := getString(row, &iName)
		description := getString(row, &iDescription)
		customsTerritoriesRaw := getString(row, &iCustomsTerritories)
		customsTerritories, err := prepareCustomsTerritories(customsTerritoriesRaw)
		if err != nil {
			log.Fatalln(err)
		}

		category := getStringPtr(row, iCategory)
		subcategory := getStringPtr(row, iSubcategory)
		countryOfOrigin := getStringPtr(row, iCountryOfOrigin)
		grossMass, err := getFloatPtr(row, iGrossMass)
		if err != nil {
			log.Fatalf("invalid gross mass for item %q\n", id)
		}
		netMass, err := getFloatPtr(row, iNetMass)
		if err != nil {
			log.Fatalf("invalid net mass for item %q\n", id)
		}
		weightUnit := getStringPtr(row, iWeightUnit)
		model := getStringPtr(row, iModel)

		imp.ImportItems[i] = ImportItemRequest{
			ID:                 id,
			Name:               name,
			Description:        description,
			Category:           category,
			Subcategory:        subcategory,
			CountryOfOrigin:    countryOfOrigin,
			GrossMass:          grossMass,
			NetMass:            netMass,
			WeightUnit:         weightUnit,
			CustomsTerritories: customsTerritories,
			Model:              model,
		}
	}

	importLocation, err := sendImportRequest(imp, url, apiKey)
	if err != nil {
		log.Fatalln(err)
	}

	err = waitForProcessing(url, importLocation, apiKey, timeout)
	if err != nil {
		if errors.Is(err, ErrFailed) {
			log.Fatalln("error processing import")
		} else {
			log.Fatalln(err)
		}
	}

	importResponse, err := getImportResponse(url, importLocation, apiKey)
	if err != nil {
		log.Fatalln(err)
	}

	for _, item := range importResponse.ImportItems {
		rowIndex, row := getRowByItemID(rows, iID, item.ID)
		if row == nil {
			log.Fatalf("error processing import response, row with item id %q is not found\n", item.ID)
		}
		// Excel is 1 indexed. The first data row is 2 (the heading is 1).
		rowIndex++

		// Append columns to match the length of the headings row.
		for len(row) < len(headings)+1 {
			row = append(row, "")
		}

		taricEU := item.getTaricByTerritory(customsTerritoryEU)
		taricNO := item.getTaricByTerritory(customsTerritoryNO)
		row[iResultEU] = taricEU.Code
		row[iResultNO] = taricNO.Code

		err = file.SetSheetRow("Sheet1", fmt.Sprintf("A%d", rowIndex), &row)
		if err != nil {
			log.Fatalln(err)
		}
	}

	err = file.SaveAs(outputPath)
	if err != nil {
		log.Fatalln(err)
	}

	fmt.Printf("\n\nDone!\nThe output is written to: %q\n", outputPath)
}

func getRowByItemID(rows [][]string, idIndex int, itemID string) (int, []string) {
	for i, row := range rows {
		if itemID == row[idIndex] {
			return i, row
		}
	}

	return 0, nil
}

func getMandatoryColumnIndex(row []string, name string) (int, error) {
	index := getColumnIndex(row, name)
	if index == nil {
		return 0, fmt.Errorf(`provided file has no %q column`, name)
	}

	return *index, nil
}

func getColumnIndex(row []string, name string) *int {
	for i, rowName := range row {
		if strings.EqualFold(name, strings.TrimSpace(rowName)) {
			return &i
		}
	}

	return nil
}

func getString(row []string, i *int) string {
	if i == nil {
		return ""
	}

	return row[*i]
}

func getStringPtr(row []string, i *int) *string {
	if i == nil {
		return nil
	}

	return &row[*i]
}

func getFloatPtr(row []string, i *int) (*float64, error) {
	if i == nil {
		return nil, nil
	}
	value := row[*i]
	if value == "" {
		return nil, nil
	}

	f, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return nil, err
	}

	return &f, err
}

func prepareCustomsTerritories(customsTerritories string) ([]string, error) {
	var result []string
	for _, territory := range strings.Split(customsTerritories, ",") {
		territory = strings.TrimSpace(strings.ToLower(territory))
		if !slices.Contains(allowedCustomsTerritories, territory) {
			return nil, fmt.Errorf("customs territory %q is not supported", territory)
		}
		result = append(result, territory)
	}

	return result, nil
}
