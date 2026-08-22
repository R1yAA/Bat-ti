package handlers

import (
	"net/http"
	"strings"

	"github.com/R1yAA/Bat-ti/app/database"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// P5 — categories, occasion tags and the single data wipe.

// uncategorizedCategoryName is the reassignment target BR-13 names. The row is
// seeded by the first migration and protected from deletion by a trigger.
const uncategorizedCategoryName = "Uncategorized"

// deleteAllDataConfirmationPhrase is what the user must type to wipe their
// data (BR-14).
const deleteAllDataConfirmationPhrase = "DELETE ALL MY DATA"

type categoryResponse struct {
	CategoryID   uuid.UUID `json:"category_id"`
	CategoryName string    `json:"category_name"`
	IsSystem     bool      `json:"is_system"`
	UsageCount   int64     `json:"usage_count"`
}

func (server *Server) handleListCategories(context *gin.Context) {
	categoryRows, err := server.queries.ListCategories(context)
	if err != nil {
		server.respondDatabaseError(context, err, "categories")
		return
	}
	categories := make([]categoryResponse, 0, len(categoryRows))
	for _, categoryRow := range categoryRows {
		categories = append(categories, categoryResponse{
			CategoryID:   categoryRow.CategoryID,
			CategoryName: categoryRow.CategoryName,
			IsSystem:     categoryRow.IsSystem,
			UsageCount:   categoryRow.UsageCount,
		})
	}
	context.JSON(http.StatusOK, gin.H{"categories": categories})
}

type namedRequest struct {
	Name string `json:"name"`
}

func (server *Server) handleCreateCategory(context *gin.Context) {
	categoryName, ok := readName(context)
	if !ok {
		return
	}
	categoryRow, err := server.queries.CreateCategory(context, categoryName)
	if err != nil {
		server.respondDatabaseError(context, err, "category")
		return
	}
	context.JSON(http.StatusCreated, categoryResponse{
		CategoryID:   categoryRow.CategoryID,
		CategoryName: categoryRow.CategoryName,
		IsSystem:     categoryRow.IsSystem,
	})
}

func (server *Server) handleRenameCategory(context *gin.Context) {
	categoryID, ok := parseUUIDParam(context, "categoryID")
	if !ok {
		return
	}
	categoryName, ok := readName(context)
	if !ok {
		return
	}
	categoryRow, err := server.queries.RenameCategory(context, database.RenameCategoryParams{
		CategoryID:   categoryID,
		CategoryName: categoryName,
	})
	if err != nil {
		server.respondDatabaseError(context, err, "category")
		return
	}
	context.JSON(http.StatusOK, categoryResponse{
		CategoryID:   categoryRow.CategoryID,
		CategoryName: categoryRow.CategoryName,
		IsSystem:     categoryRow.IsSystem,
	})
}

// BR-13: deleting a category is never blocked. Everything tagged with it picks
// up "Uncategorized" first, inside one transaction, so no order item is left
// without a category and no reference dangles.
func (server *Server) handleDeleteCategory(context *gin.Context) {
	categoryID, ok := parseUUIDParam(context, "categoryID")
	if !ok {
		return
	}

	transaction, err := server.pool.Begin(context)
	if err != nil {
		server.respondDatabaseError(context, err, "category")
		return
	}
	defer transaction.Rollback(context)
	transactionalQueries := server.queries.WithTx(transaction)

	if err := transactionalQueries.ReassignCategoryTagsToUncategorized(context,
		categoryID); err != nil {
		server.respondDatabaseError(context, err, "category")
		return
	}
	if err := transactionalQueries.DeleteCategoryTagsForCategory(context,
		categoryID); err != nil {
		server.respondDatabaseError(context, err, "category")
		return
	}
	if err := transactionalQueries.DeleteCategory(context, categoryID); err != nil {
		// The trigger guarding "Uncategorized" surfaces here.
		respondError(context, http.StatusConflict,
			"this category cannot be deleted: "+err.Error())
		return
	}
	if err := transaction.Commit(context); err != nil {
		server.respondDatabaseError(context, err, "category")
		return
	}
	context.Status(http.StatusNoContent)
}

type occasionTagResponse struct {
	OccasionTagID uuid.UUID `json:"occasion_tag_id"`
	TagName       string    `json:"tag_name"`
	UsageCount    int64     `json:"usage_count"`
}

func (server *Server) handleListOccasionTags(context *gin.Context) {
	tagRows, err := server.queries.ListOccasionTags(context)
	if err != nil {
		server.respondDatabaseError(context, err, "occasion tags")
		return
	}
	tags := make([]occasionTagResponse, 0, len(tagRows))
	for _, tagRow := range tagRows {
		tags = append(tags, occasionTagResponse{
			OccasionTagID: tagRow.OccasionTagID,
			TagName:       tagRow.TagName,
			UsageCount:    tagRow.UsageCount,
		})
	}
	context.JSON(http.StatusOK, gin.H{"occasion_tags": tags})
}

func (server *Server) handleCreateOccasionTag(context *gin.Context) {
	tagName, ok := readName(context)
	if !ok {
		return
	}
	tagRow, err := server.queries.CreateOccasionTag(context, tagName)
	if err != nil {
		server.respondDatabaseError(context, err, "occasion tag")
		return
	}
	context.JSON(http.StatusCreated, occasionTagResponse{
		OccasionTagID: tagRow.OccasionTagID,
		TagName:       tagRow.TagName,
	})
}

func (server *Server) handleRenameOccasionTag(context *gin.Context) {
	tagID, ok := parseUUIDParam(context, "tagID")
	if !ok {
		return
	}
	tagName, ok := readName(context)
	if !ok {
		return
	}
	tagRow, err := server.queries.RenameOccasionTag(context, database.RenameOccasionTagParams{
		OccasionTagID: tagID,
		TagName:       tagName,
	})
	if err != nil {
		server.respondDatabaseError(context, err, "occasion tag")
		return
	}
	context.JSON(http.StatusOK, occasionTagResponse{
		OccasionTagID: tagRow.OccasionTagID,
		TagName:       tagRow.TagName,
	})
}

func (server *Server) handleDeleteOccasionTag(context *gin.Context) {
	tagID, ok := parseUUIDParam(context, "tagID")
	if !ok {
		return
	}
	if err := server.queries.DeleteOccasionTag(context, tagID); err != nil {
		server.respondDatabaseError(context, err, "occasion tag")
		return
	}
	context.Status(http.StatusNoContent)
}

type deleteAllDataRequest struct {
	Confirmation string `json:"confirmation"`
}

// BR-14: one global wipe, gated by typing an exact phrase.
//
// Its scope is the user's own records — orders, comparisons, categories and
// occasion tags. Vendors, listings and price history survive: they are not the
// user's data, and price history in particular accumulates one day at a time
// and cannot be re-scraped once destroyed, while orders can be re-entered from
// receipts.
func (server *Server) handleDeleteAllData(context *gin.Context) {
	var request deleteAllDataRequest
	if err := context.ShouldBindJSON(&request); err != nil {
		respondError(context, http.StatusBadRequest, "confirmation is required")
		return
	}
	if request.Confirmation != deleteAllDataConfirmationPhrase {
		respondError(context, http.StatusBadRequest,
			"type exactly \""+deleteAllDataConfirmationPhrase+"\" to confirm")
		return
	}

	transaction, err := server.pool.Begin(context)
	if err != nil {
		server.respondDatabaseError(context, err, "data wipe")
		return
	}
	defer transaction.Rollback(context)
	transactionalQueries := server.queries.WithTx(transaction)

	// Order items and tag joins fall with their parents by cascade.
	for _, wipeStep := range []func() error{
		func() error { return transactionalQueries.DeleteAllOrderEntries(context) },
		func() error { return transactionalQueries.DeleteAllCompareEntries(context) },
		func() error { return transactionalQueries.DeleteAllOccasionTags(context) },
		func() error { return transactionalQueries.DeleteAllNonSystemCategories(context) },
	} {
		if err := wipeStep(); err != nil {
			server.respondDatabaseError(context, err, "data wipe")
			return
		}
	}
	if err := transaction.Commit(context); err != nil {
		server.respondDatabaseError(context, err, "data wipe")
		return
	}

	server.logger.Warn("all user data deleted on request")
	context.JSON(http.StatusOK, gin.H{
		"deleted": []string{"order entries", "order items", "compare entries",
			"occasion tags", "custom categories"},
		"kept": []string{"vendors", "listings", "variants", "moq tiers", "price history"},
	})
}

func readName(context *gin.Context) (string, bool) {
	var request namedRequest
	if err := context.ShouldBindJSON(&request); err != nil {
		respondError(context, http.StatusBadRequest, "name is required")
		return "", false
	}
	trimmedName := strings.TrimSpace(request.Name)
	if trimmedName == "" {
		respondError(context, http.StatusBadRequest, "name cannot be blank")
		return "", false
	}
	return trimmedName, true
}
