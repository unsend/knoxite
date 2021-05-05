/*
 * knoxite
 *     Copyright (c) 2021, Christian Muehlhaeuser <muesli@gmail.com>
 *     Copyright (c) 2021, Nicolas Martin <penguwin@penguwin.eu>
 *     TODO
 *
 *   For license see LICENSE
 */

package onedrive

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sync"

	"github.com/goh-chunlin/go-onedrive/onedrive"
	"github.com/knoxite/knoxite"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/microsoft"
)

// Test custom drive item
type DriveItem struct {
	Name string `json:"name"`
	Size int    `json:"size"`
}

// OnedriveStorage stores data on the onedrive $PRODUCTNAME cloud
type OnedriveStorage struct {
	url url.URL
	odc *onedrive.Client
	knoxite.StorageFilesystem
}

func init() {
	knoxite.RegisterStorageBackend(&OnedriveStorage{})
}

// NewBackend returns a OnedriveStorage backend.
func (*OnedriveStorage) NewBackend(u url.URL) (knoxite.Backend, error) {
	// parse the user and password from the url
	fmt.Printf(u.User.Username())
	if u.User == nil || u.User.Username() == "" {
		return &OnedriveStorage{}, knoxite.ErrInvalidUsername
	}
	pw, pwexist := u.User.Password()
	if !pwexist {
		return &OnedriveStorage{}, knoxite.ErrInvalidPassword
	}

	// TODO: initialize an onedrive api `client` here
	ctx := context.Background()
	conf := &oauth2.Config{
		ClientID:     u.User.Username(),
		ClientSecret: pw,
		Scopes:       []string{"files.readwrite"},
		Endpoint:     microsoft.AzureADEndpoint(""),
		RedirectURL:  "http://localhost:8080/welcome.html",
		// RedirectURL: "https://login.microsoftonline.com/common/oauth2/nativeclient",
	}

	// Redirect user to consent page to ask for permission
	// for the scopes specified above.
	reqURL := conf.AuthCodeURL("state", oauth2.AccessTypeOffline)
	fmt.Printf("Visit the URL for the auth dialog: %v\n", reqURL)

	httpServerExitDone := &sync.WaitGroup{}

	httpServerExitDone.Add(1)
	startHttpServer(httpServerExitDone)

	// Use the authorization code that is pushed to the redirect
	// URL. Exchange will do the handshake to retrieve the
	// initial access token. The HTTP Client returned by
	// conf.Client will refresh the token as necessary.
	var code string
	fmt.Println("Please enter the exchange code:")
	if _, err := fmt.Scan(&code); err != nil {
		log.Fatal(err)
	}

	tok, err := conf.Exchange(ctx, code)
	if err != nil {
		log.Fatal(err)
	}

	client := conf.Client(ctx, tok)

	odc := onedrive.NewClient(client)

	backend := OnedriveStorage{
		url: u,
		odc: odc,
	}

	fs, err := knoxite.NewStorageFilesystem(u.Path, &backend)
	if err != nil {
		return &OnedriveStorage{}, err
	}
	backend.StorageFilesystem = fs

	return &backend, nil
}

// Location returns the type and location of the repository.
func (backend *OnedriveStorage) Location() string {
	return backend.url.String()
}

// Close the backend.
func (backend *OnedriveStorage) Close() error {
	// NOTE: You may not need to close anything for onedrive
	return nil
}

// Protocols returns the Protocol Schemes supported by this backend.
func (backend *OnedriveStorage) Protocols() []string {
	return []string{"onedrive"}
}

// Description returns a user-friendly description for this backend.
func (backend *OnedriveStorage) Description() string {
	return "Onedrive Storage"
}

// AvailableSpace returns the free space on this backend.
func (backend *OnedriveStorage) AvailableSpace() (uint64, error) {
	// TODO: return the available space on onedrive cloud
	return 0, nil
}

// CreatePath creates a dir including all its parent dirs, when required.
func (backend *OnedriveStorage) CreatePath(path string) error {
	// TODO: create the given path on onedrive cloud
	return nil
}

// Stat returns the size of a file.
func (backend *OnedriveStorage) Stat(path string) (uint64, error) {
	// TODO: return the size for the file from the given path.
	return 0, nil
}

// ReadFile reads a file from onedrive cloud.
func (backend *OnedriveStorage) ReadFile(path string) ([]byte, error) {
	// TODO: read the file for the given path on onedrive cloud and return the
	// data.
	return nil, nil
}

// WriteFile write files on onedrive cloud.
func (backend *OnedriveStorage) WriteFile(path string, data []byte) (size uint64, err error) {
	// TODO: write the data in the given path on the onedrive cloud
	return 0, nil
}

// DeleteFile deletes a file from onedrive cloud.
func (backend *OnedriveStorage) DeleteFile(path string) error {
	// TODO: delete the file on the given path
	return nil
}

func startHttpServer(wg *sync.WaitGroup) {
	m := http.NewServeMux()
	server := &http.Server{Addr: ":8080", Handler: m}
	fs := http.FileServer(http.Dir("storage/onedrive/"))
	m.Handle("/welcome.html", fs)

	go func() {
		defer wg.Done()

		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("ListenAndServe(): %v", err)
		}
	}()
}
