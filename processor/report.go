package processor

import (
	"bufio"
	"fmt"
	"os"
)

func (proc *Processor) GenerateReport() {
	var writer *bufio.Writer

	if proc.OutputWriter != nil {
		writer = proc.OutputWriter
	} else {
		writer = bufio.NewWriter(os.Stdout)
	}

	defer writer.Flush()

	// Don't generate empty JSON list - produce empty file instead

	empty := true

	for _, frames := range proc.groups {
		if len(frames) < 2 {
			continue
		}

		empty = false
		break
	}

	if empty {
		return
	}

	fmt.Fprint(writer, "[")
	bucketsep := "\n  "

	for bucket, frames := range proc.groups {
		if len(frames) < 2 {
			continue
		}

		fmt.Fprintf(writer, "%s{\n    \"bucket\": %d,\n    \"files\": [\n", bucketsep, bucket)
		bucketsep = ",\n  "

		for ii, frameID := range frames {
			filesep := ","

			if ii == len(frames)-1 {
				filesep = ""
			}

			videoFile, _ := proc.state.GetImageFile(frameID)
			fmt.Fprintf(writer, "      \"%s\"%s\n", videoFile, filesep)
		}

		fmt.Fprint(writer, "    ]\n  }")
	}

	fmt.Fprint(writer, "\n]")
}
