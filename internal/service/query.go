package service

import (
 modeldevv1 "github.com/zhangzhe-ctrl/ani-modeldev-service/api/ani/modeldev/v1"
 "github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
)

type Query struct {
 modeldevv1.UnimplementedModelDevQueryServiceServer
 repository biz.ArtifactQueryRepository
 signer biz.ArtifactSigner
}

func NewQuery(repository biz.ArtifactQueryRepository, signer biz.ArtifactSigner) *Query {
 return &Query{repository: repository, signer: signer}
}
