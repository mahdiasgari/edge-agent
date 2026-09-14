package allocator

import (
	"fmt"
	"hash/fnv"
	"strings"
)

func normalize(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimSuffix(value, ".")

	return strings.ToLower(value)
}

func hash(value string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(value))
	return h.Sum32()
}

func hashString(value string) string {
	return fmt.Sprintf(
		"%08x",
		hash(value),
	)
}
