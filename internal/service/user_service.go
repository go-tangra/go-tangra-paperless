package service

import (
	"context"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/tx7do/kratos-bootstrap/bootstrap"

	adminstubpb "github.com/go-tangra/go-tangra-common/gen/go/common/admin_stub/v1"
	"github.com/go-tangra/go-tangra-paperless/internal/client"
	paperlesspb "github.com/go-tangra/go-tangra-paperless/gen/go/paperless/service/v1"
)

type UserService struct {
	paperlesspb.UnimplementedPaperlessUserServiceServer

	log         *log.Helper
	adminClient *client.AdminClient
}

func NewUserService(ctx *bootstrap.Context, adminClient *client.AdminClient) *UserService {
	return &UserService{
		log:         ctx.NewLoggerHelper("paperless/service/user"),
		adminClient: adminClient,
	}
}

func (s *UserService) ListUsers(ctx context.Context, req *paperlesspb.ListPaperlessUsersRequest) (*paperlesspb.ListPaperlessUsersResponse, error) {
	resp, err := s.adminClient.ListUsers(ctx)
	if err != nil {
		s.log.Errorf("Failed to list users from admin-service: %v", err)
		return nil, err
	}

	items := make([]*paperlesspb.PaperlessUser, 0, len(resp.Items))
	for _, u := range resp.Items {
		if u.Status != nil && u.GetStatus() == adminstubpb.AdminUser_PENDING {
			continue
		}
		items = append(items, &paperlesspb.PaperlessUser{
			Id:       u.Id,
			Username: u.Username,
			Realname: u.Realname,
			Email:    u.Email,
		})
	}

	return &paperlesspb.ListPaperlessUsersResponse{
		Items: items,
		Total: int32(len(items)),
	}, nil
}

func (s *UserService) ListRoles(ctx context.Context, req *paperlesspb.ListPaperlessRolesRequest) (*paperlesspb.ListPaperlessRolesResponse, error) {
	resp, err := s.adminClient.ListRoles(ctx)
	if err != nil {
		s.log.Errorf("Failed to list roles from admin-service: %v", err)
		return nil, err
	}

	items := make([]*paperlesspb.PaperlessRole, 0, len(resp.Items))
	for _, r := range resp.Items {
		items = append(items, &paperlesspb.PaperlessRole{
			Id:          r.Id,
			Name:        r.Name,
			Code:        r.Code,
			Description: r.Description,
		})
	}

	return &paperlesspb.ListPaperlessRolesResponse{
		Items: items,
		Total: int32(len(items)),
	}, nil
}
