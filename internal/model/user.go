package model

import "time"

type User struct {
	ID           string
	Username     string
	DisplayName  string
	AvatarURL    string
	Bio          string
	PasswordHash string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type Profile struct {
	ID          string    `json:"id"`
	Username    string    `json:"username"`
	DisplayName string    `json:"displayName"`
	AvatarURL   string    `json:"avatarUrl"`
	Bio         string    `json:"bio"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

func (u User) Public() Profile {
	return Profile{
		ID:          u.ID,
		Username:    u.Username,
		DisplayName: u.DisplayName,
		AvatarURL:   u.AvatarURL,
		Bio:         u.Bio,
		CreatedAt:   u.CreatedAt.UTC(),
		UpdatedAt:   u.UpdatedAt.UTC(),
	}
}
