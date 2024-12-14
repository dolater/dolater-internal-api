package server

import (
	"errors"
	"log"
	"net/http"
	"time"

	"firebase.google.com/go/v4/messaging"
	"github.com/dolater/dolater-internal-api/db"
	api "github.com/dolater/dolater-internal-api/generated"
	"github.com/dolater/dolater-internal-api/model"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// const parameterName = "task_pool_capacity"
const capacity = 604800

func (s *Server) CheckCapacity(c *gin.Context) {
	db, err := db.GormDB("public")
	if err != nil {
		message := err.Error()
		c.JSON(http.StatusInternalServerError, api.Error{
			Message: &message,
		})
		return
	}
	defer func() {
		sqldb, err := db.DB()
		if err != nil {
			log.Println("Failed to close database connection")
		}
		sqldb.Close()
	}()

	now := time.Now()

	var taskPools []model.TaskPool
	if err := db.
		Where(&model.TaskPool{
			Type: "taskPoolTypeActive",
		}).
		Find(&taskPools).
		Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			message := err.Error()
			c.JSON(http.StatusInternalServerError, api.Error{
				Message: &message,
			})
			return
		}
	}

	for _, taskPool := range taskPools {
		var tasks []model.Task
		if err := db.
			Where(&model.Task{
				PoolId: &taskPool.Id,
			}).
			Order("created_at ASC").
			Find(&tasks).
			Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				message := err.Error()
				c.JSON(http.StatusInternalServerError, api.Error{
					Message: &message,
				})
				return
			}
		}

		elapsedSeconds := 0.0
		var i int
		for j, task := range tasks {
			if elapsedSeconds > capacity {
				i = j
				break
			}
			elapsedSeconds += now.Sub(task.CreatedAt).Seconds()
		}

		if i >= len(tasks)-1 {
			continue
		}

		var fcmTokens []model.FCMToken
		if err := db.
			Where(&model.FCMToken{
				UserId: taskPool.OwnerId,
			}).
			Find(&fcmTokens).
			Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
		}
		registrationTokens := make([]string, len(fcmTokens))
		for i, fcmToken := range fcmTokens {
			registrationTokens[i] = fcmToken.RegistrationToken
		}
		overflowedTasks := tasks[i:]
		notifications := make([]model.Notification, len(overflowedTasks))
		messages := make([]*messaging.MulticastMessage, len(overflowedTasks))
		for i, task := range overflowedTasks {
			notifications[i] = model.Notification{
				Id:     uuid.New(),
				UserId: taskPool.OwnerId,
				Title:  "あとまわしリンクが溢れました!!",
				Body:   "",
				URL:    "https://dolater.kantacky.com/tasks/" + task.Id.String(),
			}
			messages[i] = &messaging.MulticastMessage{
				Data: map[string]string{
					"url": notifications[i].URL,
				},
				Notification: &messaging.Notification{
					Title: notifications[i].Title,
					Body:  notifications[i].Body,
				},
				APNS: &messaging.APNSConfig{
					Payload: &messaging.APNSPayload{
						Aps: &messaging.Aps{
							Sound: "default",
						},
					},
				},
				Tokens: registrationTokens,
			}
		}
		if err := db.
			Clauses(clause.OnConflict{
				DoNothing: true,
			}).
			Create(&notifications).
			Error; err != nil {
			message := err.Error()
			c.JSON(http.StatusInternalServerError, api.Error{
				Message: &message,
			})
			continue
		}

		if len(messages) == 0 {
			continue
		}
		client, err := s.FirebaseApp.Messaging(c)
		if err != nil {
			continue
		}
		for _, message := range messages {
			br, err := client.SendEachForMulticast(c, message)
			if err != nil {
				continue
			}
			log.Println(br)
		}
	}

	// log.Println(s.RemoteConfig.Parameters[parameterName].DefaultValue.Value)

	c.Status(http.StatusNoContent)
}
