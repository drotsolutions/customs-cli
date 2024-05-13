package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	ImportItemStatusPending    = "pending"
	ImportItemStatusProcessing = "processing"
	ImportItemStatusProcessed  = "processed"
	ImportItemStatusFailed     = "failed"
)

type ImportRequest struct {
	ImportItems []ImportItemRequest `json:"items"`
}

type ImportItemRequest struct {
	ID              string          `json:"id"`
	Name            string          `json:"name"`
	Description     string          `json:"description"`
	Category        *string         `json:"category,omitempty"`
	Subcategory     *string         `json:"subcategory,omitempty"`
	CountryOfOrigin *string         `json:"countryOfOrigin,omitempty"`
	GrossMass       *float64        `json:"grossMass,omitempty"`
	NetMass         *float64        `json:"netMass,omitempty"`
	WeightUnit      *string         `json:"weightUnit,omitempty"`
	Actions         []ActionRequest `json:"actions"` // actions to perform on the item
}

type ActionRequest struct {
	Name       string     `json:"name"`
	Parameters Parameters `json:"parameters"`
}

type Parameters struct {
	CustomsTerritories []string `json:"customsTerritories"`
	Model              *string  `json:"model,omitempty"` // Model is a non-documented internal property, don't use it.
}

type ImportStatus struct {
	Status string `json:"status"`
}

type ImportResponse struct {
	ID          string               `json:"id"`
	ImportItems []ImportItemResponse `json:"items"`
	CreatedAt   time.Time            `json:"createdAt"`
	UpdatedAt   time.Time            `json:"updatedAt"`
}

type ImportItemResponse struct {
	ID              string                   `json:"id"`
	Name            string                   `json:"name"`
	Description     string                   `json:"description"`
	Category        *string                  `json:"category,omitempty"`
	Subcategory     *string                  `json:"subcategory,omitempty"`
	CountryOfOrigin *string                  `json:"countryOfOrigin,omitempty"`
	GrossMass       *float64                 `json:"grossMass,omitempty"`
	NetMass         *float64                 `json:"netMass,omitempty"`
	WeightUnit      *string                  `json:"weightUnit,omitempty"`
	Actions         []ActionResponse         `json:"actions,omitempty"`
	Tarics          []CommodityCodesResponse `json:"commodityCodes"`
	CreatedAt       time.Time                `json:"createdAt"`
	UpdatedAt       time.Time                `json:"updatedAt"`
}

func (i ImportItemResponse) getTaricByTerritory(territory string) *CommodityCodesResponse {
	for _, taric := range i.Tarics {
		if taric.CustomsTerritory == territory {
			return &taric
		}
	}

	return nil
}

func (i ImportItemResponse) getAction(name string) *ActionResponse {
	for _, action := range i.Actions {
		if action.Name == name {
			return &action
		}
	}

	return nil
}

type ActionResponse struct {
	Name        string     `json:"name"`
	Parameters  Parameters `json:"parameters"`
	Status      string     `json:"status"`
	Error       *string    `json:"error,omitempty"`
	Attempts    int        `json:"attempts"`
	MaxAttempts int        `json:"maxAttempts"`
}

type CommodityCodesResponse struct {
	CustomsTerritory string `json:"customsTerritory"`
	Code             string `json:"code"`
}

func sendImportRequest(request ImportRequest, url, apiKey string) (string, error) {
	body, err := json.Marshal(request)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/api/v1/items/imports", url), bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Add("Authorization", prepareApiKey(apiKey))
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}

	if http.StatusCreated != res.StatusCode {
		resBody, bodyErr := io.ReadAll(res.Body)
		// Added to help with debugging. If there is an error while readying the body, ignore it because the original issue is more important.
		if bodyErr == nil {
			_ = res.Body.Close()
		}

		return "", fmt.Errorf("unexpected status code while importing items %d\n%s\n", res.StatusCode, string(resBody))
	}

	return res.Header.Get("Location"), nil
}

func getImportResponse(url, importLocation, apiKey string) (*ImportResponse, error) {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s%s", url, importLocation), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Authorization", prepareApiKey(apiKey))
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	if http.StatusOK != res.StatusCode {
		resBody, bodyErr := io.ReadAll(res.Body)
		// Added to help with debugging. If there is an error while readying the body, ignore it because the original issue is more important.
		if bodyErr == nil {
			_ = res.Body.Close()
		}

		return nil, fmt.Errorf("unexpected status code while getting an import %d\n%s\n", res.StatusCode, string(resBody))
	}

	var imp ImportResponse
	resBody, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	err = res.Body.Close()
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(resBody, &imp)
	if err != nil {
		return nil, err
	}

	return &imp, nil
}

func waitForProcessing(url, importLocation, apiKey string, timeout int) error {
	var importStatusResponse ImportStatus
	fmt.Printf("Waiting for the import job")
	for i := 0; i < timeout; i++ {
		fmt.Printf(".")
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s%s/status", url, importLocation), nil)
		if err != nil {
			return err
		}

		req.Header.Add("Authorization", prepareApiKey(apiKey))
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}

		resBody, err := io.ReadAll(res.Body)
		if err != nil {
			return err
		}
		err = res.Body.Close()
		if err != nil {
			return err
		}

		err = json.Unmarshal(resBody, &importStatusResponse)
		if err != nil {
			return err
		}

		if importStatusResponse.Status == ImportItemStatusFailed {
			return ErrFailed
		}
		if importStatusResponse.Status == ImportItemStatusProcessed {
			return nil
		}

		time.Sleep(time.Second)
	}

	return ErrNotProcessed
}

func prepareApiKey(apiKey string) string {
	return "Bearer " + strings.TrimPrefix(apiKey, "Bearer ")
}
