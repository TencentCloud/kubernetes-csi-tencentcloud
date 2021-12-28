package cfs

import (
	"os"
)

func GetSercet() (secretID, secretKey, token string, isTokenUpdate bool) {
	secretID = os.Getenv("TENCENTCLOUD_API_SECRET_ID")
	secretKey = os.Getenv("TENCENTCLOUD_API_SECRET_KEY")
	return
}


