package output

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/rodaine/table"
)

var Format = "table"

func PrintJSON(v any) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error formatting JSON: %v\n", err)
		return
	}
	fmt.Println(string(data))
}

func PrintTable(headers []string, rows [][]string) {
	if len(rows) == 0 {
		fmt.Println("No results found.")
		return
	}

	headerFmt := color.New(color.FgCyan, color.Bold).SprintfFunc()

	ifaces := make([]interface{}, len(headers))
	for i, h := range headers {
		ifaces[i] = h
	}

	tbl := table.New(ifaces...)
	tbl.WithHeaderFormatter(headerFmt)

	for _, row := range rows {
		vals := make([]interface{}, len(row))
		for i, v := range row {
			vals[i] = v
		}
		tbl.AddRow(vals...)
	}
	tbl.Print()
}

func StatusColor(status string) string {
	switch strings.ToUpper(status) {
	case "ACTIVE":
		return color.GreenString(status)
	case "PENDING":
		return color.YellowString(status)
	case "EXPIRED", "CANCELLED":
		return color.HiBlackString(status)
	case "FAILED":
		return color.RedString(status)
	case "EXPIRING":
		return color.YellowString(status)
	case "PARTIAL":
		return color.HiYellowString(status)
	case "APPLIED":
		return color.GreenString(status)
	case "REMOVED":
		return color.HiBlackString(status)
	default:
		return status
	}
}

func FormatTime(ts string) string {
	if ts == "" {
		return "-"
	}
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		t, err = time.Parse(time.RFC3339, ts)
		if err != nil {
			return ts
		}
	}
	return t.Local().Format("2006-01-02 15:04:05")
}

func FormatDuration(expiresAt string) string {
	if expiresAt == "" {
		return "-"
	}
	t, err := time.Parse(time.RFC3339Nano, expiresAt)
	if err != nil {
		t, err = time.Parse(time.RFC3339, expiresAt)
		if err != nil {
			return "-"
		}
	}
	remaining := time.Until(t)
	if remaining < 0 {
		return "expired"
	}
	hours := int(remaining.Hours())
	minutes := int(remaining.Minutes()) % 60
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}

func Success(msg string, args ...any) {
	green := color.New(color.FgGreen).SprintFunc()
	fmt.Printf("%s %s\n", green("✓"), fmt.Sprintf(msg, args...))
}

func Error(msg string, args ...any) {
	red := color.New(color.FgRed).SprintFunc()
	fmt.Fprintf(os.Stderr, "%s %s\n", red("✗"), fmt.Sprintf(msg, args...))
}

func Info(msg string, args ...any) {
	blue := color.New(color.FgBlue).SprintFunc()
	fmt.Printf("%s %s\n", blue("→"), fmt.Sprintf(msg, args...))
}
