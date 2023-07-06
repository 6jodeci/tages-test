package main

import (
	"bytes"
	"context"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"sync"

	pb "github.com/6jodeci/tages-test/pb"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	fileUploadPath = "./uploaded_files"
)

type fileServiceServer struct {
	pb.UnimplementedFileServiceServer
}

func main() {
	lis, err := net.Listen("tcp", ":8081")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	pb.RegisterFileServiceServer(s, &fileServiceServer{})

	reflection.Register(s)

	log.Println("Server started on port: 8081")
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}

func (s *fileServiceServer) UploadFile(stream pb.FileService_UploadFileServer) error {
	err := os.MkdirAll(fileUploadPath, os.ModePerm)
	if err != nil {
		return status.Errorf(codes.Internal, "failed to create file upload directory: %v", err)
	}

	var wg sync.WaitGroup
	sem := make(chan struct{}, 10) // Ограничение на 10 конкурентных запросов

	for {
		req, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return status.Errorf(codes.Unknown, "failed to receive request: %v", err)
		}

		sem <- struct{}{} 
		wg.Add(1)

		go func(req *pb.UploadRequest) {
			defer wg.Done()
			defer func() { <-sem }() 

			fileName := req.FileName
			filePath := filepath.Join(fileUploadPath, fileName)
			file, err := os.Create(filePath)
			if err != nil {
				log.Printf("failed to create file %s: %v", fileName, err)
				return
			}
			defer file.Close()

			if _, err := io.Copy(file, bytes.NewReader(req.FileData)); err != nil {
				log.Printf("failed to write file %s: %v", fileName, err)
				return
			}

			log.Printf("file %s uploaded", fileName)
		}(req)
	}

	wg.Wait()

	err = stream.SendAndClose(&pb.UploadResponse{
		Success: true,
		Message: "File(s) uploaded successfully",
	})
	if err != nil {
		return status.Errorf(codes.Unknown, "failed to send response: %v", err)
	}

	return nil
}

func (s *fileServiceServer) ListFiles(ctx context.Context, req *pb.ListFilesRequest) (*pb.ListFilesResponse, error) {
	files, err := os.ReadDir(fileUploadPath)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read file directory: %v", err)
	}

	resp := &pb.ListFilesResponse{}

	sem := make(chan struct{}, 100) // Ограничение на 100 конкурентных запросов
	var wg sync.WaitGroup

	for _, file := range files {
		sem <- struct{}{}
		wg.Add(1)

		go func(file os.DirEntry) {
			defer wg.Done()
			defer func() { <-sem }()

			info, err := file.Info()
			if err != nil {
				log.Printf("failed to get file info: %v", err)
				return
			}

			created := timestamppb.New(info.ModTime())
			updated := timestamppb.New(info.ModTime())

			resp.Files = append(resp.Files, &pb.FileInfo{
				FileName:  file.Name(),
				CreatedAt: created,
				UpdatedAt: updated,
			})
		}(file)
	}

	wg.Wait()

	return resp, nil
}

func (s *fileServiceServer) DownloadFile(req *pb.DownloadRequest, stream pb.FileService_DownloadFileServer) error {
	filePath := filepath.Join(fileUploadPath, req.FileName)
	file, err := os.Open(filePath)
	if err != nil {
		return status.Errorf(codes.NotFound, "file %s not found", req.FileName)
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return status.Errorf(codes.Unknown, "failed to get file info: %v", err)
	}

	buffer := make([]byte, fileInfo.Size())
	_, err = file.Read(buffer)
	if err != nil {
		return status.Errorf(codes.Unknown, "failed to read file: %v", err)
	}

	if err := stream.Send(&pb.DownloadResponse{
		FileData: buffer,
	}); err != nil {
		return status.Errorf(codes.Unknown, "failed to send file data: %v", err)
	}

	return nil
}
