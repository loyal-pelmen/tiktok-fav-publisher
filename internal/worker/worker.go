package worker

import (
	"fmt"
	"github.com/exceptioon/tiktok-fav-publisher/internal"
	"github.com/exceptioon/tiktok-fav-publisher/internal/tiktok"
	"go.uber.org/zap"
	"gopkg.in/telebot.v3"
	"sync"
	"time"
)

type Bot struct {
	Bot    *telebot.Bot
	ChatID int64
}

type Worker struct {
	TikTok *tiktok.ServiceApi
	TG     Bot
	Cache  internal.Cache
	WG     *sync.WaitGroup
	Log    *zap.Logger
	Tick   *time.Ticker

	QuitChan chan struct{}
}

func (w *Worker) Start() {
	defer w.WG.Done()
	go w.TG.Bot.Start()

LOOP:
	for {
		select {
		case <-w.Tick.C:
			videos, err := w.TikTok.GetLikedVideos()
			if err != nil {
				w.Log.Error("got error", zap.Error(err))
				continue
			}

			for _, video := range videos {
				if w.Cache.IsExist(video.ID) {
					continue
				}
				time.Sleep(time.Second * 5)
				err = w.TikTok.SetVideoMetadata(&video)

				w.Log.Info("processing video", zap.String("ID", video.ID), zap.String("url", video.ShareableLink),
					zap.String("download", video.DownloadLink))

				menu := &telebot.ReplyMarkup{ResizeKeyboard: true}
				menu.Inline(
					menu.Row(menu.URL("Original", video.ShareableLink)),
				)

				if len(video.Images) > 0 {
					photoAlbum := make(telebot.Album, len(video.Images))

					for i, image := range video.Images {
						photo := &telebot.Photo{File: telebot.FromURL(image)}
						if i == 0 {
							photo.Caption = fmt.Sprintf("%s", video.AuthorUsername, video.Title)
						}

						photoAlbum[i] = photo
					}

					_, err := w.TG.Bot.SendAlbum(telebot.ChatID(w.TG.ChatID), photoAlbum, menu)
					if err != nil {
						w.Log.Error("Send images", zap.Error(err), zap.String("download url", video.DownloadLink))
						continue
					}
				} else {
					_, err = w.TG.Bot.Send(telebot.ChatID(w.TG.ChatID),
						&telebot.Video{
							File:    telebot.File{FileURL: video.DownloadLink},
							Caption: fmt.Sprintf("@%s: %s", video.AuthorUsername, video.Title),
						}, menu)

					if err != nil {
						w.Log.Error("Send video", zap.Error(err), zap.String("download url", video.DownloadLink))
						continue
					}
				}

				err = w.Cache.Add(video.ID)
				if err != nil {
					w.Log.Error("Add to cache", zap.Error(err))
					continue
				}
				w.Log.Info("sent video", zap.String("id", video.ID))
			}

		case <-w.QuitChan:
			w.Tick.Stop()
			w.TG.Bot.Stop()
			break LOOP
		}
	}
}

func (w *Worker) Stop() {
	w.QuitChan <- struct{}{}
}
