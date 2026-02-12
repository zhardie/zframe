package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"time"

	"github.com/disintegration/imaging"
)

type ImmichAsset struct {
	ID string `json:"id"`
}

type AlbumResponse struct {
	Assets []ImmichAsset `json:"assets"`
}

func getPhotoHandler(w http.ResponseWriter, r *http.Request) {
	immichURL := os.Getenv("IMMICH_URL") // e.g. http://10.0.0.5:2283
	apiKey := os.Getenv("IMMICH_API_KEY")
	albumID := os.Getenv("IMMICH_ALBUM_ID")

	client := &http.Client{Timeout: 30 * time.Second}

	// 1. Get Album Data
	albumURL := fmt.Sprintf("%s/api/albums/%s", immichURL, albumID)
	req, _ := http.NewRequest("GET", albumURL, nil)
	req.Header.Set("x-api-key", apiKey)

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Network Error: %v", err)
		http.Error(w, "Connection failed", 500)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == 403 {
		log.Println("403 FORBIDDEN: Check if your API Key has 'all' or 'album.read' permissions!")
		http.Error(w, "Immich permission denied", 403)
		return
	}

	var album AlbumResponse
	if err := json.NewDecoder(resp.Body).Decode(&album); err != nil {
		log.Printf("JSON Error: %v", err)
		http.Error(w, "Failed to parse album", 500)
		return
	}

	if len(album.Assets) == 0 {
		http.Error(w, "Album is empty", 404)
		return
	}

	// 2. Pick a random photo
	rand.Seed(time.Now().UnixNano())
	asset := album.Assets[rand.Intn(len(album.Assets))]

	// 3. Fetch 'preview' (Immich converts .ARW to high-res JPG for us)
	photoURL := fmt.Sprintf("%s/api/assets/%s/thumbnail?size=preview", immichURL, asset.ID)
	log.Printf("Pulling photo: %s", asset.ID)

	photoReq, _ := http.NewRequest("GET", photoURL, nil)
	photoReq.Header.Set("x-api-key", apiKey)
	photoResp, err := client.Do(photoReq)
	if err != nil || photoResp.StatusCode != 200 {
		log.Printf("Failed to fetch image: %d", photoResp.StatusCode)
		http.Error(w, "Image fetch failed", 500)
		return
	}
	defer photoResp.Body.Close()

	// 4. Processing
	src, err := imaging.Decode(photoResp.Body)
	if err != nil {
		log.Printf("Decode Error: %v (Ensure _ image/jpeg is imported!)", err)
		http.Error(w, "Decode failed", 500)
		return
	}

	// Fill crops the image to exactly 1600x1200 without stretching
	dst := imaging.Fill(src, 1600, 1200, imaging.Center, imaging.Lanczos)

	// E-ink optimization with +20% contrast on e-ink
	dst = imaging.AdjustContrast(dst, 20.0)

	// 5. Serve it
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate") // Prevent stale images
	imaging.Encode(w, dst, imaging.JPEG, imaging.JPEGQuality(100))
}

func main() {
	http.HandleFunc("/photo", getPhotoHandler)
	log.Println("zframe is live on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
