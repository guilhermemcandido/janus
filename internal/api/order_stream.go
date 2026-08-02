package api

import (
	"fmt"
	"io"
	"log"
	"sync"

	"google.golang.org/grpc/codes"

	pb "github.com/guilhermemcandido/janus/internal/api/proto"
	"github.com/guilhermemcandido/janus/internal/types"
)

// OrderStream carries every Submit/Cancel over one long-lived stream instead of a fresh unary call
// per order. Commands run concurrently (one goroutine each), since internal/web shares one Client.
func (s *Server) OrderStream(stream pb.Exchange_OrderStreamServer) error {
	events := make(chan *pb.OrderEvent, 64)
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		for evt := range events {
			if stream.Send(evt) != nil {
				return
			}
		}
	}()

	var wg sync.WaitGroup
	for {
		cmd, err := stream.Recv()
		if err != nil {
			wg.Wait()
			close(events)
			<-writerDone
			if err == io.EOF {
				return nil
			}
			return err
		}
		wg.Add(1)
		go func(cmd *pb.OrderCommand) {
			defer wg.Done()
			evt := s.handleOrderCommand(cmd)
			select {
			case events <- evt:
			case <-stream.Context().Done():
			}
		}(cmd)
	}
}

// handleOrderCommand runs one Submit or Cancel and always returns a reply - errors ride back as an
// OrderEvent_Error, never as a returned Go error, so one bad command can't end the whole stream.
func (s *Server) handleOrderCommand(cmd *pb.OrderCommand) *pb.OrderEvent {
	switch c := cmd.Command.(type) {
	case *pb.OrderCommand_Submit:
		return s.handleSubmit(cmd.CorrelationId, c.Submit)
	case *pb.OrderCommand_Cancel:
		return s.handleCancel(cmd.CorrelationId, c.Cancel)
	default:
		return orderErrorEvent(cmd.CorrelationId, codes.InvalidArgument, "empty OrderCommand")
	}
}

func (s *Server) handleSubmit(correlationID uint64, req *pb.SubmitOrderRequest) *pb.OrderEvent {
	eng, ok := s.exchange.Lookup(req.Symbol)
	if !ok {
		return orderErrorEvent(correlationID, codes.NotFound, "market %q is not registered", req.Symbol)
	}
	order := types.NewOrder(req.Symbol, sideFromProto(req.Side), typeFromProto(req.Type), req.Price, req.Quantity)

	trades, err := eng.Submit(order)
	if err != nil {
		return orderErrorEvent(correlationID, toCode(err), "%s", err.Error())
	}
	for _, tr := range trades {
		log.Printf("trade: %s %d @ %d (maker %d, taker %d)", req.Symbol, tr.Quantity, tr.Price, tr.MakerOrderID, tr.TakerOrderID)
	}
	return &pb.OrderEvent{
		CorrelationId: correlationID,
		Event: &pb.OrderEvent_SubmitResult{SubmitResult: &pb.SubmitOrderResponse{
			Order:  orderToProto(order),
			Trades: tradesToProto(req.Symbol, trades),
		}},
	}
}

func (s *Server) handleCancel(correlationID uint64, req *pb.CancelOrderRequest) *pb.OrderEvent {
	eng, ok := s.exchange.Lookup(req.Symbol)
	if !ok {
		return orderErrorEvent(correlationID, codes.NotFound, "market %q is not registered", req.Symbol)
	}

	order, err := eng.Cancel(req.OrderId)
	if err != nil {
		return orderErrorEvent(correlationID, toCode(err), "%s", err.Error())
	}
	return &pb.OrderEvent{
		CorrelationId: correlationID,
		Event:         &pb.OrderEvent_CancelResult{CancelResult: &pb.CancelOrderResponse{Order: orderToProto(order)}},
	}
}

func orderErrorEvent(correlationID uint64, code codes.Code, format string, args ...any) *pb.OrderEvent {
	return &pb.OrderEvent{
		CorrelationId: correlationID,
		Event: &pb.OrderEvent_Error{Error: &pb.OrderError{
			Code:    uint32(code),
			Message: fmt.Sprintf(format, args...),
		}},
	}
}
