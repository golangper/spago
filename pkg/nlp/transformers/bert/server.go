// Copyright 2020 spaGO Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bert

//go:generate protoc --go_out=Mgrpc/service_config/service_config.proto=/internal/proto/grpc_service_config:.  --go-grpc_out=Mgrpc/service_config/service_config.proto=/internal/proto/grpc_service_config:. --go_opt=paths=source_relative --go-grpc_opt=paths=source_relative grpcapi/bert.proto

import (
	"bytes"
	"encoding/json"
	"github.com/nlpodyssey/spago/pkg/webui/bertclassification"
	"net/http"
	"sort"

	"github.com/nlpodyssey/spago/pkg/ml/ag"
	"github.com/nlpodyssey/spago/pkg/nlp/tokenizers/wordpiecetokenizer"
	"github.com/nlpodyssey/spago/pkg/nlp/transformers/bert/grpcapi"
	"github.com/nlpodyssey/spago/pkg/utils"
	"github.com/nlpodyssey/spago/pkg/utils/grpcutils"
	"github.com/nlpodyssey/spago/pkg/utils/httputils"
	"github.com/nlpodyssey/spago/pkg/webui/bertqa"
)

// TODO: This code needs to be refactored. Pull requests are welcome!

// Server contains everything needed to run a BERT server.
type Server struct {
	model *Model

	// UnimplementedBERTServer must be embedded to have forward compatible implementations for gRPC.
	grpcapi.UnimplementedBERTServer
}

// NewServer returns Server objects.
func NewServer(model *Model) *Server {
	return &Server{
		model: model,
	}
}

// StartDefaultServer is used to start a basic BERT HTTP server.
// If you want more control of the HTTP server you can run your own
// HTTP router using the public handler functions
func (s *Server) StartDefaultServer(address, grpcAddress, tlsCert, tlsKey string, tlsDisable bool) {
	mux := http.NewServeMux()
	mux.HandleFunc("/bert-qa-ui", bertqa.Handler)
	mux.HandleFunc("/bert-classify-ui", bertclassification.Handler)
	mux.HandleFunc("/discriminate", s.DiscriminateHandler)
	mux.HandleFunc("/predict", s.PredictHandler)
	mux.HandleFunc("/answer", s.QaHandler)
	mux.HandleFunc("/tag", s.LabelerHandler)
	mux.HandleFunc("/classify", s.ClassifyHandler)
	mux.HandleFunc("/te", s.TextualEntailmentHandler)

	go httputils.RunHTTPServer(address, tlsDisable, tlsCert, tlsKey, mux)

	grpcServer := grpcutils.NewGRPCServer(tlsDisable, tlsCert, tlsKey)
	grpcapi.RegisterBERTServer(grpcServer, s)
	grpcutils.RunGRPCServer(grpcAddress, grpcServer)
}

type Body struct {
	Text string `json:"text"`
}

type QABody struct {
	Question string `json:"question"`
	Passage  string `json:"passage"`
}

func pad(words []string) []string {
	leftPad := wordpiecetokenizer.DefaultClassToken
	rightPad := wordpiecetokenizer.DefaultSequenceSeparator
	return append([]string{leftPad}, append(words, rightPad)...)
}

const DefaultRealLabel = "REAL"
const DefaultFakeLabel = "FAKE"
const DefaultPredictedLabel = "PREDICTED"

type Answer struct {
	Text       string  `json:"text"`
	Start      int     `json:"start"`
	End        int     `json:"end"`
	Confidence float64 `json:"confidence"`
}

type AnswerSlice []Answer

func (p AnswerSlice) Len() int           { return len(p) }
func (p AnswerSlice) Less(i, j int) bool { return p[i].Confidence < p[j].Confidence }
func (p AnswerSlice) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
func (p AnswerSlice) Sort()              { sort.Sort(p) }

type QuestionAnsweringResponse struct {
	Answers AnswerSlice `json:"answers"`
	// Took is the number of milliseconds it took the server to execute the request.
	Took int64 `json:"took"`
}

func (r *QuestionAnsweringResponse) Dump(pretty bool) ([]byte, error) {
	buf := bytes.NewBufferString("")
	enc := json.NewEncoder(buf)
	if pretty {
		enc.SetIndent("", "    ")
	}
	enc.SetEscapeHTML(true)
	err := enc.Encode(r)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

const defaultMaxAnswerLength = 20     // TODO: from options
const defaultMinConfidence = 0.1      // TODO: from options
const defaultMaxCandidateLogits = 3.0 // TODO: from options
const defaultMaxAnswers = 3           // TODO: from options

func extractScores(logits []ag.Node) []float64 {
	scores := make([]float64, len(logits))
	for i, node := range logits {
		scores[i] = node.ScalarValue()
	}
	return scores
}

func getBestIndices(logits []float64, size int) []int {
	s := utils.NewFloat64Slice(logits...)
	sort.Sort(sort.Reverse(s))
	if len(s.Indices) < size {
		return s.Indices
	}
	return s.Indices[:size]
}

type Response struct {
	Tokens []Token `json:"tokens"`
	// Took is the number of milliseconds it took the server to execute the request.
	Took int64 `json:"took"`
}

type Token struct {
	Text  string `json:"text"`
	Start int    `json:"start"`
	End   int    `json:"end"`
	Label string `json:"label"`
}

func (r *Response) Dump(pretty bool) ([]byte, error) {
	buf := bytes.NewBufferString("")
	enc := json.NewEncoder(buf)
	if pretty {
		enc.SetIndent("", "    ")
	}
	enc.SetEscapeHTML(true)
	err := enc.Encode(r)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
