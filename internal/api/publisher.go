package api

import (
	"context"
	"encoding/json"
	"time"

	"github.com/d11nn/woms/internal/domain"
	"github.com/segmentio/kafka-go"
)

type ScheduleJobPublisher interface {
	PublishScheduleJob(ctx context.Context, job domain.ScheduleJob) error
	Close() error
}

type NoopScheduleJobPublisher struct{}

func (NoopScheduleJobPublisher) PublishScheduleJob(context.Context, domain.ScheduleJob) error {
	return nil
}

func (NoopScheduleJobPublisher) Close() error {
	return nil
}

type KafkaScheduleJobPublisher struct {
	writer *kafka.Writer
}

func NewKafkaScheduleJobPublisher(brokers []string, topic string) *KafkaScheduleJobPublisher {
	return &KafkaScheduleJobPublisher{
		writer: &kafka.Writer{
			Addr:                   kafka.TCP(brokers...),
			Topic:                  topic,
			AllowAutoTopicCreation: true,
			BatchTimeout:           50 * time.Millisecond,
		},
	}
}

func (p *KafkaScheduleJobPublisher) PublishScheduleJob(ctx context.Context, job domain.ScheduleJob) error {
	payload, err := json.Marshal(job)
	if err != nil {
		return err
	}
	return p.writer.WriteMessages(ctx, kafka.Message{
		Key:   []byte(job.ID),
		Value: payload,
		Time:  time.Now().UTC(),
	})
}

func (p *KafkaScheduleJobPublisher) Close() error {
	return p.writer.Close()
}
