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
)

func init() {
	flag.BoolVar(&help, "help", false, "")
	flag.StringVar(&apiKey, "api-key", "", "")
	flag.StringVar(&url, "url", defaultURL, "")
	flag.StringVar(&outputPath, "output", defaultOutput, "")
}

func main() {
	flag.Parse()
	if help {
		fmt.Printf(`	Import items from an excel file and generate customs codes. The generated customs codes will be written to the provided output file (default %q).

	Options:
		--api-key	API key used for the authentication and authorization
		--url		URL of the server (default %q)
		--output	writer output to the file (default %q)
		--help		display this help and exit
`, defaultOutput, defaultURL, defaultOutput)

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
	iID, err := getMandatoryColumnIndex("id", headings)
	if err != nil {
		log.Fatalln(err)
	}
	iName, err := getMandatoryColumnIndex("name", headings)
	if err != nil {
		log.Fatalln(err)
	}
	iDescription, err := getMandatoryColumnIndex("description", headings)
	if err != nil {
		log.Fatalln(err)
	}
	iCustomsTerritories, err := getMandatoryColumnIndex("customsTerritories", headings)
	if err != nil {
		log.Fatalln(err)
	}

	iCategory := getColumnIndex("category", headings)
	iSubcategory := getColumnIndex("subcategory", headings)
	iCountryOfOrigin := getColumnIndex("countryOfOrigin", headings)
	iGrossMass := getColumnIndex("grossMass", headings)
	iNetMass := getColumnIndex("netMass", headings)
	iWeightUnit := getColumnIndex("weightUnit", headings)
	iModel := getColumnIndex("model", headings)

	// If result columns don't exist we will create them.
	iActualEU := getColumnIndex("actualEU", headings)
	if iActualEU == nil {
		headings = append(headings, "actualEU")
		iActualEU = getColumnIndex("actualEU", headings)
	}
	iActualNO := getColumnIndex("actualNO", headings)
	if iActualNO == nil {
		headings = append(headings, "actualNO")
		iActualNO = getColumnIndex("actualNO", headings)
	}

	// Write headings to the output, because we have modified them by appending the result columns.
	err = file.SetSheetRow("Sheet1", "A1", &headings)
	if err != nil {
		log.Fatalln(err)
	}

	imp := ImportRequest{
		ImportItems: make([]ImportItemRequest, len(rows[1:])),
	}

	for i, row := range rows[1:] {
		id := getString(&iID, row)
		name := getString(&iName, row)
		description := getString(&iDescription, row)
		customsTerritoriesRaw := getString(&iCustomsTerritories, row)
		customsTerritories, err := prepareCustomsTerritories(customsTerritoriesRaw)
		if err != nil {
			log.Fatalln(err)
		}

		category := getStringPtr(iCategory, row)
		subcategory := getStringPtr(iSubcategory, row)
		countryOfOrigin := getStringPtr(iCountryOfOrigin, row)
		grossMass, err := getFloatPtr(iGrossMass, row)
		if err != nil {
			log.Fatalf("invalid gross mass for item %q\n", id)
		}
		netMass, err := getFloatPtr(iNetMass, row)
		if err != nil {
			log.Fatalf("invalid net mass for item %q\n", id)
		}
		weightUnit := getStringPtr(iWeightUnit, row)
		model := getStringPtr(iModel, row)

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

	err = waitForProcessing(url, importLocation, apiKey)
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
		rowIndex, row := getRowByItemID(item.ID, iID, rows)
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
		row[*iActualEU] = taricEU.Code
		row[*iActualNO] = taricNO.Code

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

func getRowByItemID(id string, idIndex int, rows [][]string) (int, []string) {
	for i, row := range rows {
		if id == row[idIndex] {
			return i, row
		}
	}

	return 0, nil
}

func getMandatoryColumnIndex(name string, row []string) (int, error) {
	index := getColumnIndex(name, row)
	if index == nil {
		return 0, fmt.Errorf(`provided file has no %q column`, name)
	}

	return *index, nil
}

func getColumnIndex(name string, row []string) *int {
	for i, rowName := range row {
		if strings.EqualFold(name, strings.TrimSpace(rowName)) {
			return &i
		}
	}

	return nil
}

func getString(i *int, row []string) string {
	if i == nil {
		return ""
	}

	return row[*i]
}

func getStringPtr(i *int, row []string) *string {
	if i == nil {
		return nil
	}

	return &row[*i]
}

func getFloatPtr(i *int, row []string) (*float64, error) {
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
