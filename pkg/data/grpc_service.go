package data

import (
	"context"
	"encoding/json"
	"time"

	pb "github.com/quidditch/quidditch/pkg/common/proto"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// DataService implements the gRPC DataService
type DataService struct {
	pb.UnimplementedDataServiceServer
	node   *DataNode
	logger *zap.Logger
}

// NewDataService creates a new data service
func NewDataService(node *DataNode, logger *zap.Logger) *DataService {
	return &DataService{
		node:   node,
		logger: logger,
	}
}

// CreateShard creates a new shard on this data node
func (s *DataService) CreateShard(ctx context.Context, req *pb.CreateShardRequest) (*pb.CreateShardResponse, error) {
	s.logger.Info("CreateShard request",
		zap.String("index", req.IndexName),
		zap.Int32("shard_id", req.ShardId),
		zap.Bool("is_primary", req.IsPrimary))

	// Validate request
	if req.IndexName == "" {
		return nil, status.Error(codes.InvalidArgument, "index name is required")
	}
	if req.ShardId < 0 {
		return nil, status.Error(codes.InvalidArgument, "shard_id must be non-negative")
	}

	// Create shard
	if err := s.node.shards.CreateShard(ctx, req.IndexName, req.ShardId, req.IsPrimary); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create shard: %v", err)
	}

	shardKey := shardKey(req.IndexName, req.ShardId)

	return &pb.CreateShardResponse{
		Acknowledged: true,
		ShardKey:     shardKey,
	}, nil
}

// DeleteShard deletes a shard from this data node
func (s *DataService) DeleteShard(ctx context.Context, req *pb.DeleteShardRequest) (*pb.DeleteShardResponse, error) {
	s.logger.Info("DeleteShard request",
		zap.String("index", req.IndexName),
		zap.Int32("shard_id", req.ShardId))

	// Validate request
	if req.IndexName == "" {
		return nil, status.Error(codes.InvalidArgument, "index name is required")
	}

	// Delete shard
	if err := s.node.shards.DeleteShard(ctx, req.IndexName, req.ShardId); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to delete shard: %v", err)
	}

	return &pb.DeleteShardResponse{
		Acknowledged: true,
	}, nil
}

// GetShardInfo returns information about a shard
func (s *DataService) GetShardInfo(ctx context.Context, req *pb.GetShardInfoRequest) (*pb.ShardInfo, error) {
	s.logger.Debug("GetShardInfo request",
		zap.String("index", req.IndexName),
		zap.Int32("shard_id", req.ShardId))

	// Get shard
	shard, err := s.node.shards.GetShard(req.IndexName, req.ShardId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "shard not found: %v", err)
	}

	// Convert to proto
	return &pb.ShardInfo{
		IndexName:   shard.IndexName,
		ShardId:     shard.ShardID,
		IsPrimary:   shard.IsPrimary,
		State:       s.convertShardStateToProto(shard.State),
		DocsCount:   shard.DocsCount,
		SizeBytes:   shard.SizeBytes,
		CreatedAt:   timestamppb.New(time.Now()), // TODO: Store creation time
		LastUpdated: timestamppb.New(time.Now()),
	}, nil
}

// RefreshShard makes recently indexed documents searchable
func (s *DataService) RefreshShard(ctx context.Context, req *pb.RefreshShardRequest) (*pb.RefreshShardResponse, error) {
	s.logger.Debug("RefreshShard request",
		zap.String("index", req.IndexName),
		zap.Int32("shard_id", req.ShardId))

	// Get shard
	shard, err := s.node.shards.GetShard(req.IndexName, req.ShardId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "shard not found: %v", err)
	}

	// Refresh shard
	if err := shard.Refresh(); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to refresh shard: %v", err)
	}

	return &pb.RefreshShardResponse{
		Acknowledged: true,
	}, nil
}

// FlushShard flushes shard data to disk
func (s *DataService) FlushShard(ctx context.Context, req *pb.FlushShardRequest) (*pb.FlushShardResponse, error) {
	s.logger.Debug("FlushShard request",
		zap.String("index", req.IndexName),
		zap.Int32("shard_id", req.ShardId))

	// Get shard
	shard, err := s.node.shards.GetShard(req.IndexName, req.ShardId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "shard not found: %v", err)
	}

	// Flush shard
	if err := shard.Flush(); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to flush shard: %v", err)
	}

	return &pb.FlushShardResponse{
		Acknowledged: true,
	}, nil
}

// IndexDocument indexes a document into a shard
func (s *DataService) IndexDocument(ctx context.Context, req *pb.IndexDocumentRequest) (*pb.IndexDocumentResponse, error) {
	s.logger.Debug("IndexDocument request",
		zap.String("index", req.IndexName),
		zap.Int32("shard_id", req.ShardId),
		zap.String("doc_id", req.DocId))

	// Validate request
	if req.IndexName == "" {
		return nil, status.Error(codes.InvalidArgument, "index name is required")
	}
	if req.DocId == "" {
		return nil, status.Error(codes.InvalidArgument, "doc_id is required")
	}
	if req.Document == nil {
		return nil, status.Error(codes.InvalidArgument, "document is required")
	}

	// Get shard
	shard, err := s.node.shards.GetShard(req.IndexName, req.ShardId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "shard not found: %v", err)
	}

	// Convert protobuf Struct to map
	doc := req.Document.AsMap()

	// Index document
	if err := shard.IndexDocument(ctx, req.DocId, doc); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to index document: %v", err)
	}

	return &pb.IndexDocumentResponse{
		Acknowledged: true,
		DocId:        req.DocId,
		Version:      1, // TODO: Implement versioning
	}, nil
}

// GetDocument retrieves a document by ID
func (s *DataService) GetDocument(ctx context.Context, req *pb.GetDocumentRequest) (*pb.GetDocumentResponse, error) {
	s.logger.Debug("GetDocument request",
		zap.String("index", req.IndexName),
		zap.Int32("shard_id", req.ShardId),
		zap.String("doc_id", req.DocId))

	// Validate request
	if req.IndexName == "" {
		return nil, status.Error(codes.InvalidArgument, "index name is required")
	}
	if req.DocId == "" {
		return nil, status.Error(codes.InvalidArgument, "doc_id is required")
	}

	// Get shard
	shard, err := s.node.shards.GetShard(req.IndexName, req.ShardId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "shard not found: %v", err)
	}

	// Get document
	doc, err := shard.GetDocument(ctx, req.DocId)
	if err != nil {
		// Document not found
		return &pb.GetDocumentResponse{
			Found: false,
			DocId: req.DocId,
		}, nil
	}

	// Convert map to protobuf Struct
	docStruct, err := structpb.NewStruct(doc)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to convert document: %v", err)
	}

	return &pb.GetDocumentResponse{
		Found:    true,
		DocId:    req.DocId,
		Document: docStruct,
		Version:  1, // TODO: Implement versioning
	}, nil
}

// DeleteDocument deletes a document by ID
func (s *DataService) DeleteDocument(ctx context.Context, req *pb.DeleteDocumentRequest) (*pb.DeleteDocumentResponse, error) {
	s.logger.Debug("DeleteDocument request",
		zap.String("index", req.IndexName),
		zap.Int32("shard_id", req.ShardId),
		zap.String("doc_id", req.DocId))

	// Validate request
	if req.IndexName == "" {
		return nil, status.Error(codes.InvalidArgument, "index name is required")
	}
	if req.DocId == "" {
		return nil, status.Error(codes.InvalidArgument, "doc_id is required")
	}

	// Get shard
	shard, err := s.node.shards.GetShard(req.IndexName, req.ShardId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "shard not found: %v", err)
	}

	// Delete document
	if err := shard.DeleteDocument(ctx, req.DocId); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to delete document: %v", err)
	}

	return &pb.DeleteDocumentResponse{
		Acknowledged: true,
		Found:        true, // TODO: Check if document existed
	}, nil
}

// BulkIndex indexes multiple documents in a single request
func (s *DataService) BulkIndex(ctx context.Context, req *pb.BulkIndexRequest) (*pb.BulkIndexResponse, error) {
	s.logger.Debug("BulkIndex request",
		zap.String("index", req.IndexName),
		zap.Int32("shard_id", req.ShardId),
		zap.Int("items", len(req.Items)))

	// Validate request
	if req.IndexName == "" {
		return nil, status.Error(codes.InvalidArgument, "index name is required")
	}
	if len(req.Items) == 0 {
		return nil, status.Error(codes.InvalidArgument, "items are required")
	}

	// Get shard
	shard, err := s.node.shards.GetShard(req.IndexName, req.ShardId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "shard not found: %v", err)
	}

	startTime := time.Now()
	hasErrors := false
	items := make([]*pb.BulkIndexItemResponse, 0, len(req.Items))

	// Index each document
	for _, item := range req.Items {
		doc := item.Document.AsMap()
		err := shard.IndexDocument(ctx, item.DocId, doc)

		itemResp := &pb.BulkIndexItemResponse{
			DocId: item.DocId,
		}

		if err != nil {
			hasErrors = true
			itemResp.Acknowledged = false
			itemResp.Error = err.Error()
		} else {
			itemResp.Acknowledged = true
		}

		items = append(items, itemResp)
	}

	tookMillis := time.Since(startTime).Milliseconds()

	return &pb.BulkIndexResponse{
		HasErrors:   hasErrors,
		Items:       items,
		TookMillis:  tookMillis,
	}, nil
}

// Search executes a search query on a shard
func (s *DataService) Search(ctx context.Context, req *pb.SearchRequest) (*pb.SearchResponse, error) {
	s.logger.Debug("Search request",
		zap.String("index", req.IndexName),
		zap.Int32("shard_id", req.ShardId))

	// Validate request
	if req.IndexName == "" {
		return nil, status.Error(codes.InvalidArgument, "index name is required")
	}
	if req.Query == nil {
		return nil, status.Error(codes.InvalidArgument, "query is required")
	}

	// Get shard
	shard, err := s.node.shards.GetShard(req.IndexName, req.ShardId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "shard not found: %v", err)
	}

	startTime := time.Now()

	// Execute search (UDF queries are embedded in req.Query JSON)
	result, err := shard.Search(ctx, req.Query)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "search failed: %v", err)
	}

	tookMillis := time.Since(startTime).Milliseconds()

	// Convert search result to proto
	hits := make([]*pb.SearchHit, 0, len(result.Hits))
	for _, hit := range result.Hits {
		// Convert document to protobuf Struct
		docStruct, err := structpb.NewStruct(hit.Source)
		if err != nil {
			s.logger.Error("Failed to convert document", zap.Error(err))
			continue
		}

		hits = append(hits, &pb.SearchHit{
			Id:     hit.ID,
			Score:  hit.Score,
			Source: docStruct,
		})
	}

	return &pb.SearchResponse{
		TookMillis: tookMillis,
		TimedOut:   false,
		Shards: &pb.ShardSearchStats{
			Total:      1,
			Successful: 1,
			Failed:     0,
		},
		Hits: &pb.SearchHits{
			Total: &pb.TotalHits{
				Value:    result.TotalHits,
				Relation: "eq",
			},
			MaxScore: result.MaxScore,
			Hits:     hits,
		},
	}, nil
}

// Count returns the count of documents matching a query
func (s *DataService) Count(ctx context.Context, req *pb.CountRequest) (*pb.CountResponse, error) {
	s.logger.Debug("Count request",
		zap.String("index", req.IndexName),
		zap.Int32("shard_id", req.ShardId))

	// Validate request
	if req.IndexName == "" {
		return nil, status.Error(codes.InvalidArgument, "index name is required")
	}

	// Get shard
	shard, err := s.node.shards.GetShard(req.IndexName, req.ShardId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "shard not found: %v", err)
	}

	// For now, return document count
	// TODO: Implement query-based counting
	_ = req.Query

	stats := shard.Stats()

	return &pb.CountResponse{
		Count: stats.DocsCount,
	}, nil
}

// GetShardStats returns statistics for a specific shard
func (s *DataService) GetShardStats(ctx context.Context, req *pb.GetShardStatsRequest) (*pb.ShardStats, error) {
	s.logger.Debug("GetShardStats request",
		zap.String("index", req.IndexName),
		zap.Int32("shard_id", req.ShardId))

	// Get shard
	shard, err := s.node.shards.GetShard(req.IndexName, req.ShardId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "shard not found: %v", err)
	}

	// Get stats
	stats := shard.Stats()

	return &pb.ShardStats{
		IndexName:               stats.IndexName,
		ShardId:                 stats.ShardID,
		IsPrimary:               stats.IsPrimary,
		DocsCount:               stats.DocsCount,
		DocsDeleted:             0, // TODO: Track deleted docs
		SizeBytes:               stats.SizeBytes,
		SearchQueriesTotal:      0, // TODO: Track query metrics
		SearchQueriesTimeMillis: 0,
		IndexingTotal:           0, // TODO: Track indexing metrics
		IndexingTimeMillis:      0,
	}, nil
}

// GetNodeStats returns statistics for the entire node
func (s *DataService) GetNodeStats(ctx context.Context, req *pb.GetNodeStatsRequest) (*pb.DataNodeStats, error) {
	s.logger.Debug("GetNodeStats request",
		zap.Bool("include_shards", req.IncludeShards))

	// Get all shards
	shards := s.node.shards.List()

	// Aggregate stats
	var totalDocs, totalSize int64
	shardStats := make([]*pb.ShardStats, 0, len(shards))

	for _, shard := range shards {
		stats := shard.Stats()
		totalDocs += stats.DocsCount
		totalSize += stats.SizeBytes

		if req.IncludeShards {
			shardStats = append(shardStats, &pb.ShardStats{
				IndexName:               stats.IndexName,
				ShardId:                 stats.ShardID,
				IsPrimary:               stats.IsPrimary,
				DocsCount:               stats.DocsCount,
				DocsDeleted:             0, // TODO: Track deleted docs
				SizeBytes:               stats.SizeBytes,
				SearchQueriesTotal:      0, // TODO: Track query metrics
				SearchQueriesTimeMillis: 0,
				IndexingTotal:           0, // TODO: Track indexing metrics
				IndexingTimeMillis:      0,
			})
		}
	}

	// TODO: Get actual CPU, memory, disk usage
	nodeStats := &pb.DataNodeStats{
		NodeId:              s.node.cfg.NodeID,
		TotalShards:         int32(len(shards)),
		TotalDocs:           totalDocs,
		TotalSizeBytes:      totalSize,
		CpuUsagePercent:     0.0,  // TODO: Implement
		MemoryUsagePercent:  0.0,  // TODO: Implement
		DiskUsagePercent:    0.0,  // TODO: Implement
		UptimeSeconds:       0,    // TODO: Track uptime
		Shards:              shardStats,
	}

	return nodeStats, nil
}

// Helper functions

func (s *DataService) convertShardStateToProto(state ShardState) pb.ShardInfo_ShardState {
	switch state {
	case ShardStateInitializing:
		return pb.ShardInfo_SHARD_STATE_INITIALIZING
	case ShardStateStarted:
		return pb.ShardInfo_SHARD_STATE_STARTED
	case ShardStateRelocating:
		return pb.ShardInfo_SHARD_STATE_RELOCATING
	case ShardStateClosed:
		return pb.ShardInfo_SHARD_STATE_CLOSED
	default:
		return pb.ShardInfo_SHARD_STATE_UNKNOWN
	}
}

// Helper function for document conversion
func convertDocumentToJSON(doc map[string]interface{}) ([]byte, error) {
	return json.Marshal(doc)
}

func convertJSONToDocument(data []byte) (map[string]interface{}, error) {
	var doc map[string]interface{}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	return doc, nil
}
