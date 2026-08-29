package agent

import (
	"fmt"
	"strings"
)

// Render formats a complete result for stdout.
func Render(path string, result Result, truncated []int) string {
	var output strings.Builder
	fmt.Fprintf(&output, "文件: %s\n段数: %d\n\n", path, len(result.Items))
	if len(result.Items) == 0 {
		output.WriteString("无有效段落\n")
		return output.String()
	}

	truncatedSet := make(map[int]struct{}, len(truncated))
	for _, paragraph := range truncated {
		truncatedSet[paragraph] = struct{}{}
	}
	for index, item := range result.Items {
		fmt.Fprintf(&output, "%d. ", index+1)
		if _, ok := truncatedSet[index+1]; ok {
			output.WriteString("[截断] ")
		}
		output.WriteString(item)
		output.WriteByte('\n')
	}
	return output.String()
}
