package utils

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/fatih/color"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

func PrintPretty(data interface{}) {
	// MarshalIndent membuat format JSON dengan spasi (indentasi)
	prettyJSON, err := json.MarshalIndent(data, "", "    ")
	if err != nil {
		fmt.Println("Gagal mencetak struct:", err)
		return
	}
	fmt.Println(string(prettyJSON))
}

func PrintLogInfo(username *string, statusCode int, functionName string, err *error) {
	var logColor string

	switch statusCode {
	case http.StatusOK, http.StatusCreated, http.StatusAccepted:
		logColor = Green
	case http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound:
		logColor = Yellow
	case http.StatusInternalServerError, http.StatusNotImplemented, http.StatusBadGateway, http.StatusServiceUnavailable:
		logColor = Red
	default:
		logColor = Reset
	}

	user := "Unknown"
	if username != nil {
		user = *username
	}

	if err != nil && *err != nil {
		log.Error().Msg(fmt.Sprintf("User: %s | Status: %s | Function: %s | Error: %v", user, ColorStatus(statusCode), functionName, *err))
		fmt.Printf("%sUser: %s | Status: %s | Function: %s | Error: %v%s\n", logColor, user, ColorStatus(statusCode), functionName, *err, Reset)
		return
	}
	log.Info().Msg(fmt.Sprintf("User: %s | Status: %s | Function: %s", user, ColorStatus(statusCode), functionName))
	fmt.Printf("%sUser: %s | Status: %s | Function: %s%s\n", logColor, user, ColorStatus(statusCode), functionName, Reset)
}

func GetAPIHitter(c *gin.Context) string {
	apiHitterVal, _ := c.Get("name")      // ini masih interface{}
	apiHitter, _ := apiHitterVal.(string) // type assertion jadi string
	if apiHitterVal == nil {
		apiHitter = "unknown"
		PrintLogInfo(nil, 401, "Get API Hitter - Get Admin Name", nil)
	}
	return apiHitter
}

func PrintDTO(structName string, dto interface{}) {
	cyan := color.New(color.FgCyan).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()

	jsonData, err := json.MarshalIndent(dto, "", "  ")
	if err != nil {
		fmt.Printf("❌ Error marshaling %s: %v\n", structName, err)
		return
	}

	fmt.Printf("%s %s:\n%s\n",
		cyan("📋"),
		yellow(structName),
		string(jsonData))
}
