package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"

	ghAPI "github.com/cli/go-gh/v2/pkg/api"
)

// uploadPolicyRequest is the JSON body sent to GitHub's upload policies endpoint.
type uploadPolicyRequest struct {
	Name          string `json:"name"`
	Size          int    `json:"size"`
	ContentType   string `json:"content_type"`
	RepositoryID  int    `json:"repository_id"`
}

// uploadPolicyResponse is the JSON response from GitHub's upload policies endpoint.
type uploadPolicyResponse struct {
	UploadURL      string            `json:"upload_url"`
	FormData       map[string]string `json:"form"`
	Asset          uploadAsset       `json:"asset"`
	UploadAuthType string            `json:"upload_authenticity_token"`
}

type uploadAsset struct {
	Href string `json:"href"`
	ID   string `json:"id"`
}

// UploadImage uploads image data to GitHub and returns the URL.
// It uses GitHub's user-content upload endpoint to host the image.
func (c *Client) UploadImage(ctx context.Context, data []byte, filename string) (string, error) {
	repoID, err := c.getRepoID(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get repository ID: %w", err)
	}

	httpClient, err := ghAPI.DefaultHTTPClient()
	if err != nil {
		return "", fmt.Errorf("failed to create HTTP client: %w", err)
	}

	// Step 1: Request upload policy
	policyReq := uploadPolicyRequest{
		Name:         filename,
		Size:         len(data),
		ContentType:  "image/png",
		RepositoryID: repoID,
	}

	policyBody, err := json.Marshal(policyReq)
	if err != nil {
		return "", fmt.Errorf("failed to marshal upload policy request: %w", err)
	}

	policyURL := fmt.Sprintf("https://github.com/%s/%s/upload/policies/assets", c.owner, c.repo)
	slog.Debug("requesting upload policy", "url", policyURL, "filename", filename, "size", len(data))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, policyURL, bytes.NewReader(policyBody))
	if err != nil {
		return "", fmt.Errorf("failed to create policy request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to request upload policy: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		slog.Error("upload policy request failed", "status", resp.StatusCode, "body", string(body))
		return "", fmt.Errorf("upload policy request failed with status %d", resp.StatusCode)
	}

	var policyResp uploadPolicyResponse
	if err := json.NewDecoder(resp.Body).Decode(&policyResp); err != nil {
		return "", fmt.Errorf("failed to decode upload policy response: %w", err)
	}

	if policyResp.UploadURL == "" {
		return "", fmt.Errorf("upload policy response missing upload_url")
	}

	slog.Debug("got upload policy", "uploadURL", policyResp.UploadURL, "assetHref", policyResp.Asset.Href)

	// Step 2: Upload the file using multipart form
	var formBuf bytes.Buffer
	writer := multipart.NewWriter(&formBuf)

	// Write form fields from policy response first
	for k, v := range policyResp.FormData {
		if err := writer.WriteField(k, v); err != nil {
			return "", fmt.Errorf("failed to write form field %s: %w", k, err)
		}
	}

	// Write the file last (S3 requires file to be the last field)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return "", fmt.Errorf("failed to create form file: %w", err)
	}
	if _, err := part.Write(data); err != nil {
		return "", fmt.Errorf("failed to write file data: %w", err)
	}
	writer.Close()

	uploadReq, err := http.NewRequestWithContext(ctx, http.MethodPost, policyResp.UploadURL, &formBuf)
	if err != nil {
		return "", fmt.Errorf("failed to create upload request: %w", err)
	}
	uploadReq.Header.Set("Content-Type", writer.FormDataContentType())

	uploadResp, err := httpClient.Do(uploadReq)
	if err != nil {
		return "", fmt.Errorf("failed to upload image: %w", err)
	}
	defer uploadResp.Body.Close()

	// S3 returns 201 Created or 204 No Content on success
	if uploadResp.StatusCode != http.StatusCreated && uploadResp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(uploadResp.Body)
		slog.Error("image upload failed", "status", uploadResp.StatusCode, "body", string(body))
		return "", fmt.Errorf("image upload failed with status %d", uploadResp.StatusCode)
	}

	assetURL := policyResp.Asset.Href
	slog.Info("image uploaded", "url", assetURL)
	return assetURL, nil
}

// getRepoID fetches the numeric repository ID from the GitHub API.
func (c *Client) getRepoID(ctx context.Context) (int, error) {
	var resp struct {
		ID int `json:"id"`
	}
	endpoint := fmt.Sprintf("repos/%s/%s", c.owner, c.repo)
	if err := c.rest.Get(endpoint, &resp); err != nil {
		return 0, fmt.Errorf("failed to fetch repo: %w", err)
	}
	return resp.ID, nil
}
