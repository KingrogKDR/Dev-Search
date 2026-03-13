package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/KingrogKDR/Dev-Search/queues"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type MinioStore struct {
	Client *minio.Client
	Bucket string
}

type PageMeta struct {
	URL       string `json:"url"`
	Source    string `json:"source"`
	Simhash   uint64 `json:"simhash"`
	Timestamp int64  `json:"timestamp"`
}

func NewMinioStore(endpoint, username, password, bucket string, useSSL bool) (*MinioStore, error) {
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(username, password, ""),
		Secure: useSSL,
	})

	if err != nil {
		return nil, err
	}

	store := &MinioStore{
		Client: client,
		Bucket: bucket,
	}

	return store, nil
}

func (m *MinioStore) EnsureBucket(ctx context.Context) error {

	exists, err := m.Client.BucketExists(ctx, m.Bucket)
	if err != nil {
		return err
	}

	if !exists {
		return m.Client.MakeBucket(ctx, m.Bucket, minio.MakeBucketOptions{})
	}

	return nil
}

func (m *MinioStore) StoreRawData(ctx context.Context, raw []byte, url string, source string, hash uint64) (string, error) {
	var contentPath string
	var contentType string

	if source == "github" {
		contentPath = fmt.Sprintf("md/%d.md", hash)
		contentType = "text/markdown"
	} else {
		contentPath = fmt.Sprintf("html/%d.html", hash)
		contentType = "text/html"
	}

	_, err := m.Client.PutObject(
		ctx, m.Bucket, contentPath, bytes.NewReader(raw), int64(len(raw)), minio.PutObjectOptions{
			ContentType: contentType,
		},
	)

	if err != nil {
		return "", err
	}

	meta := PageMeta{
		URL:       url,
		Source:    source,
		Simhash:   hash,
		Timestamp: time.Now().Unix(),
	}
	metaBytes, _ := json.Marshal(meta)
	metaPath := fmt.Sprintf("domain-meta/%d.json", hash)

	_, err = m.Client.PutObject(ctx, m.Bucket, metaPath, bytes.NewReader(metaBytes), int64(len(metaBytes)), minio.PutObjectOptions{
		ContentType: "application/json",
	})

	return contentPath, err
}

func (m *MinioStore) StoreTextData(ctx context.Context, text string, hash uint64, urlMeta *queues.UrlMeta) error {
	contentPath := fmt.Sprintf("text/%d", hash)
	textBytes := []byte(text)

	_, err := m.Client.PutObject(ctx, m.Bucket, contentPath, bytes.NewReader(textBytes), int64(len(text)), minio.PutObjectOptions{
		ContentType: "text",
	})
	if err != nil {
		return err
	}

	urlMetaBytes, _ := json.Marshal(urlMeta)
	metaPath := fmt.Sprintf("url-meta/%d.json", hash)

	_, err = m.Client.PutObject(ctx, m.Bucket, metaPath, bytes.NewReader(urlMetaBytes), int64(len(urlMetaBytes)), minio.PutObjectOptions{
		ContentType: "application/json",
	})

	return err
}

func (m *MinioStore) GetObject(ctx context.Context, objectName string) ([]byte, error) {

	obj, err := m.Client.GetObject(ctx, m.Bucket, objectName, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}

	data, err := io.ReadAll(obj)
	if err != nil {
		return nil, err
	}

	return data, nil
}
