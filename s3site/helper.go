package s3site

import (
	"bufio"
	"crypto/md5"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/mholt/archiver"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
)

var tempDir string = "/tmp/s3site"

func cleanTmp() error {
	return os.RemoveAll(tempDir)
}

func prepareTmp() error {
	if _, err := os.Stat(tempDir); os.IsNotExist(err) {
		return os.Mkdir(tempDir, 0755)
	}

	return nil
}

func listS3Objects(bucket string, meta interface{}) (*s3.ListObjectsV2Output, error) {
	sess := meta.(*session.Session)
	s3conn := s3.New(sess)

	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
	}

	return s3conn.ListObjectsV2(input)
}

func deleteAllObjects(bucket string, meta interface{}) error {
	listObjectResponse, err := listS3Objects(bucket, meta)

	if err != nil {
		return err
	}

	var keys []string
	for _, object := range listObjectResponse.Contents {
		keys = append(keys, *object.Key)
	}

	return deleteObjects(bucket, keys, meta)
}

func bulkUploadS3Objects(fileMap map[string]fileInfo, bucket string, meta interface{}) error {
	sess := meta.(*session.Session)

	uploader := s3manager.NewUploader(sess)

	for _, fileInfo := range fileMap {
		file, openErr := os.Open(fileInfo.FullPath)
		if openErr != nil {
			return openErr
		}

		reader := bufio.NewReader(file)

		uploadInput := &s3manager.UploadInput{
			Bucket: &bucket,
			Key:    &fileInfo.RelativePath,
			Body:   reader,
		}

		_, uploaderErr := uploader.Upload(uploadInput)
		if uploaderErr != nil {
			return uploaderErr
		}
	}

	return nil
}

func deleteObjects(bucket string, keys []string, meta interface{}) error {
	sess := meta.(*session.Session)
	svc := s3.New(sess)

	// objectIdentifiers := []*s3.ObjectIdentifier{}

	// for _, key := range keys {
	// 	objectIdentifier := s3.ObjectIdentifier{
	// 		Key: &key,
	// 	}
	// 	log.Printf("[DEBUG] Deleting key. bucket=%s, key=%s", bucket, key)
	// 	objectIdentifiers = append(objectIdentifiers, &objectIdentifier)
	// }

	// delete := s3.Delete{
	// 	Objects: objectIdentifiers,
	// }

	// _, err := svc.DeleteObjects(&s3.DeleteObjectsInput{
	// 	Bucket: &bucket,
	// 	Delete: &delete,
	// })

	// if err != nil {
	// 	return err
	// }

	// DeleteObjects API doesn't seem to work right
	for _, key := range keys {
		log.Printf("[DEBUG] Deleting key. bucket=%s, key=%s", bucket, key)
		_, err := svc.DeleteObject(&s3.DeleteObjectInput{
			Bucket: &bucket,
			Key:    &key,
		})

		if err != nil {
			return err
		}
	}

	return nil
}

func getFileMd5(path string) (string, error) {
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return "", err
	}

	size := int64(len(content))
	contentToHash := content
	parts := 0

	if int64(size) > partSize {
		var pos int64 = 0
		contentToHash = make([]byte, 0)
		for size > pos {
			endpos := pos + partSize
			if endpos >= size {
				endpos = size
			}
			hash := md5.Sum(content[pos:endpos])
			contentToHash = append(contentToHash, hash[:]...)
			pos += partSize
			parts += 1
		}
	}

	hash := md5.Sum(contentToHash)
	etag := fmt.Sprintf("%x", hash)
	if parts > 0 {
		etag += fmt.Sprintf("-%d", parts)
	}
	return etag, nil
}

func cleanS3ETag(eTag string) string {
	return strings.Trim(eTag, "\\\"")
}

func extractArchive(archive string) ([]fileInfo, error) {
	log.Printf("[DEBUG] Extracting archive. path=%s", archive)

	extractedDir := fmt.Sprintf("%s/site/", tempDir)
	if err := archiver.Zip.Open(archive, extractedDir); err != nil {
		return nil, err
	}

	var fileList []fileInfo
	if err := filepath.Walk(extractedDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		fileList = append(fileList, fileInfo{
			FullPath:     path,
			RelativePath: strings.TrimPrefix(path, extractedDir),
			FileInfo:     info,
		})

		return nil
	}); err != nil {
		return nil, err
	}

	return fileList, nil
}

type fileInfo struct {
	FullPath     string
	RelativePath string
	FileInfo     os.FileInfo
	Hash         string
}

func encodeKey(key string) string {
	return strings.Replace(key, ".", "%%", -1)
}

func decodeKey(key string) string {
	return strings.Replace(key, "%%", ".", -1)
}
