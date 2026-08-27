package game_library

import (
	"context"
	"errors"
	"io"

	pb "github.com/movsb/gm/protocols/go/proto"
)

type GRPCSource struct {
	Client pb.BlobServiceClient
}

func (s GRPCSource) Open(ctx context.Context, blobID int32) (io.ReadCloser, error) {
	streamContext, cancel := context.WithCancel(ctx)
	stream, err := s.Client.GetBlob(streamContext, &pb.GetBlobRequest{BlobId: blobID})
	if err != nil {
		cancel()
		return nil, err
	}
	message, err := stream.Recv()
	if err == io.EOF {
		cancel()
		return nil, errors.New("blob stream ended before metadata")
	}
	if err != nil {
		cancel()
		return nil, err
	}
	if message.GetBlob() == nil {
		cancel()
		return nil, errors.New("first blob frame is not metadata")
	}
	return &grpcBlobReader{stream: stream, cancel: cancel}, nil
}

type grpcBlobReader struct {
	stream  pb.BlobService_GetBlobClient
	cancel  context.CancelFunc
	pending []byte
	closed  bool
}

func (r *grpcBlobReader) Read(buffer []byte) (int, error) {
	if r.closed {
		return 0, errors.New("blob reader is closed")
	}
	if len(buffer) == 0 {
		return 0, nil
	}
	for {
		if len(r.pending) > 0 {
			count := copy(buffer, r.pending)
			r.pending = r.pending[count:]
			return count, nil
		}
		message, err := r.stream.Recv()
		if err != nil {
			return 0, err
		}
		if message.GetBlob() != nil {
			return 0, errors.New("unexpected blob metadata frame")
		}
		r.pending = message.GetData()
	}
}

func (r *grpcBlobReader) Close() error {
	if !r.closed {
		r.closed = true
		r.cancel()
	}
	return nil
}
