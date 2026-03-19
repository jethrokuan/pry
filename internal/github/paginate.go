package github

import "fmt"

const restPageSize = 100

// paginateREST fetches all pages from a paginated GitHub REST endpoint.
// endpointFmt must contain a single %d verb for the page number,
// e.g. "repos/o/r/pulls/1/files?per_page=100&page=%d".
func paginateREST[T any](rest restClient, endpointFmt string) ([]T, error) {
	var all []T
	page := 1

	for {
		var batch []T
		err := rest.Get(fmt.Sprintf(endpointFmt, page), &batch)
		if err != nil {
			return nil, err
		}

		all = append(all, batch...)

		if len(batch) < restPageSize {
			break
		}
		page++
	}

	return all, nil
}
