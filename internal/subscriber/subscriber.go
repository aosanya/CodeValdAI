// Package subscriber connects CodeValdAI to CodeValdPubSub. On startup it
// registers a subscription for the "work.task.status.changed" topic, then
// polls for new events and logs acknowledgement of each one received.
package subscriber

import (
	"context"
	"log"
	"time"

	pb "github.com/aosanya/CodeValdPubSub/gen/go/codevaldpubsub/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	topic        = "work.task.status.changed"
	subscriberID = "codevaldai"
	pollInterval = 10 * time.Second
)

// Subscriber holds the gRPC client and subscription state.
type Subscriber struct {
	conn   *grpc.ClientConn
	client pb.PubSubServiceClient
	subID  string
	seen   map[string]struct{}
}

// New dials the PubSub gRPC server and returns a Subscriber.
func New(addr string) (*Subscriber, error) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	return &Subscriber{
		conn:   conn,
		client: pb.NewPubSubServiceClient(conn),
		seen:   make(map[string]struct{}),
	}, nil
}

// Close releases the underlying gRPC connection.
func (s *Subscriber) Close() {
	s.conn.Close()
}

// Run registers the subscription (idempotent) then polls for new events until
// ctx is cancelled. Must be called inside a goroutine.
func (s *Subscriber) Run(ctx context.Context, agencyID string) {
	if err := s.register(ctx, agencyID); err != nil {
		log.Printf("codevaldai: subscriber: register failed: %v — will retry on next poll", err)
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if s.subID == "" {
				if err := s.register(ctx, agencyID); err != nil {
					log.Printf("codevaldai: subscriber: register retry failed: %v", err)
					continue
				}
			}
			s.poll(ctx, agencyID)
		}
	}
}

func (s *Subscriber) register(ctx context.Context, agencyID string) error {
	resp, err := s.client.Subscribe(ctx, &pb.SubscribeRequest{
		AgencyId:          agencyID,
		SubscriberId:      subscriberID,
		SubscriberService: subscriberID,
		TopicPattern:      topic,
	})
	if err != nil {
		return err
	}
	s.subID = resp.Subscription.Id
	log.Printf("codevaldai: subscriber: registered subscription id=%s topic=%s", s.subID, topic)
	return nil
}

func (s *Subscriber) poll(ctx context.Context, agencyID string) {
	resp, err := s.client.QueryEvents(ctx, &pb.QueryEventsRequest{
		AgencyId: agencyID,
		Topic:    topic,
		Limit:    50,
	})
	if err != nil {
		log.Printf("codevaldai: subscriber: poll error: %v", err)
		return
	}

	for _, evt := range resp.Events {
		if _, already := s.seen[evt.Id]; already {
			continue
		}
		s.seen[evt.Id] = struct{}{}
		log.Printf("codevaldai: subscriber: ACK event_id=%s topic=%s payload=%s", evt.Id, evt.Topic, evt.Payload)
	}
}
