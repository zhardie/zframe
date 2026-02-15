package main

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg" // Register JPEG decoder
	"image/png"    // Register PNG encoder
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/disintegration/imaging"
)

// Spectra 6 (ACeP) typically uses these 6 primary pigments
var spectra6Palette = color.Palette{
	color.RGBA{0, 0, 0, 255},       // Black
	color.RGBA{255, 255, 255, 255}, // White
	color.RGBA{255, 0, 0, 255},     // Red
	color.RGBA{0, 255, 0, 255},     // Green
	color.RGBA{0, 0, 255, 255},     // Blue
	color.RGBA{255, 255, 0, 255},   // Yellow
}

type ImmichAsset struct {
	ID string `json:"id"`
}

type AlbumResponse struct {
	Assets []ImmichAsset `json:"assets"`
}

func getPhotoHandler(w http.ResponseWriter, r *http.Request) {
	immichURL := os.Getenv("IMMICH_URL")
	apiKey := os.Getenv("IMMICH_API_KEY")
	albumID := os.Getenv("IMMICH_ALBUM_ID")

	photoWidth, err := strconv.Atoi(os.Getenv("PHOTO_WIDTH"))
	if err != nil {
		photoWidth = 800 // Default to standard e-ink frame width
	}

	photoHeight, err := strconv.Atoi(os.Getenv("PHOTO_HEIGHT"))
	if err != nil {
		photoHeight = 480 // Default to standard e-ink frame height
	}

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

	if resp.StatusCode != 200 {
		log.Printf("Immich Error: %d", resp.StatusCode)
		http.Error(w, "Immich API Error", resp.StatusCode)
		return
	}

	var album AlbumResponse
	if err := json.NewDecoder(resp.Body).Decode(&album); err != nil {
		http.Error(w, "Failed to parse album", 500)
		return
	}

	if len(album.Assets) == 0 {
		http.Error(w, "Album is empty", 404)
		return
	}

	// 2. Pick random photo
	rand.Seed(time.Now().UnixNano())
	asset := album.Assets[rand.Intn(len(album.Assets))]

	// 3. Fetch 'preview'
	photoURL := fmt.Sprintf("%s/api/assets/%s/thumbnail?size=preview", immichURL, asset.ID)
	log.Printf("Processing photo: %s", asset.ID)

	photoReq, _ := http.NewRequest("GET", photoURL, nil)
	photoReq.Header.Set("x-api-key", apiKey)
	photoResp, err := client.Do(photoReq)
	if err != nil || photoResp.StatusCode != 200 {
		http.Error(w, "Image fetch failed", 500)
		return
	}
	defer photoResp.Body.Close()

	// 4. Decode & Resize
	src, err := imaging.Decode(photoResp.Body)
	if err != nil {
		log.Printf("Decode Error: %v", err)
		http.Error(w, "Decode failed", 500)
		return
	}

	// Crop/Fill to exact screen dimensions
	dst := imaging.Fill(src, photoWidth, photoHeight, imaging.Center, imaging.Lanczos)

	// --- E-INK PRE-PROCESSING ---

	// Increase Contrast: E-ink looks washed out; bump contrast to separate darks/lights
	dst = imaging.AdjustContrast(dst, 25.0)

	// Increase Saturation: Standard photos are too subtle for the limited 6-color palette.
	// Boosting saturation forces pixels closer to pure Red/Blue/Green/Yellow so the
	// dithering algorithm picks them up.
	dst = imaging.AdjustSaturation(dst, 40.0)

	// --- DITHERING (Floyd-Steinberg) ---

	// Create a new Paletted image with the Spectra 6 colors
	dithered := image.NewPaletted(dst.Bounds(), spectra6Palette)

	// Apply Floyd-Steinberg error diffusion
	draw.FloydSteinberg.Draw(dithered, dst.Bounds(), dst, image.Point{})

	// 5. Serve as PNG
	// IMPORTANT: We serve PNG because JPEG compression destroys dithering artifacts
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")

	// Encode to ResponseWriter
	if err := png.Encode(w, dithered); err != nil {
		log.Printf("Encoding Error: %v", err)
	}
}

func main() {
	http.HandleFunc("/photo", getPhotoHandler)
	log.Println("Spectra 6 Dither-Server live on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}