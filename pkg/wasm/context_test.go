package wasm

import (
	"testing"
)

func TestNewDocumentContext(t *testing.T) {
	jsonData := []byte(`{
		"id": "doc1",
		"title": "Test Document",
		"price": 99.99,
		"quantity": 10,
		"available": true,
		"metadata": {
			"category": "electronics",
			"tags": ["new", "featured"]
		}
	}`)

	ctx, err := NewDocumentContext("doc1", 1.5, jsonData)
	if err != nil {
		t.Fatalf("Failed to create context: %v", err)
	}

	if ctx.GetDocumentID() != "doc1" {
		t.Errorf("Expected document ID 'doc1', got '%s'", ctx.GetDocumentID())
	}

	if ctx.GetScore() != 1.5 {
		t.Errorf("Expected score 1.5, got %f", ctx.GetScore())
	}

	t.Log("✅ Document context created successfully")
}

func TestGetFieldString(t *testing.T) {
	jsonData := []byte(`{
		"title": "iPhone 15",
		"description": "Latest Apple smartphone"
	}`)

	ctx, err := NewDocumentContext("doc1", 1.0, jsonData)
	if err != nil {
		t.Fatalf("Failed to create context: %v", err)
	}

	// Test existing string field
	title, exists := ctx.GetFieldString("title")
	if !exists {
		t.Error("Expected title field to exist")
	}
	if title != "iPhone 15" {
		t.Errorf("Expected title 'iPhone 15', got '%s'", title)
	}

	// Test non-existent field
	_, exists = ctx.GetFieldString("nonexistent")
	if exists {
		t.Error("Expected nonexistent field to not exist")
	}

	t.Log("✅ String field access working")
}

func TestGetFieldInt64(t *testing.T) {
	jsonData := []byte(`{
		"quantity": 42,
		"views": 1000
	}`)

	ctx, err := NewDocumentContext("doc1", 1.0, jsonData)
	if err != nil {
		t.Fatalf("Failed to create context: %v", err)
	}

	// Test existing int field
	quantity, exists := ctx.GetFieldInt64("quantity")
	if !exists {
		t.Error("Expected quantity field to exist")
	}
	if quantity != 42 {
		t.Errorf("Expected quantity 42, got %d", quantity)
	}

	// Test non-existent field
	_, exists = ctx.GetFieldInt64("nonexistent")
	if exists {
		t.Error("Expected nonexistent field to not exist")
	}

	t.Log("✅ Int64 field access working")
}

func TestGetFieldFloat64(t *testing.T) {
	jsonData := []byte(`{
		"price": 99.99,
		"rating": 4.5
	}`)

	ctx, err := NewDocumentContext("doc1", 1.0, jsonData)
	if err != nil {
		t.Fatalf("Failed to create context: %v", err)
	}

	// Test existing float field
	price, exists := ctx.GetFieldFloat64("price")
	if !exists {
		t.Error("Expected price field to exist")
	}
	if price != 99.99 {
		t.Errorf("Expected price 99.99, got %f", price)
	}

	// Test non-existent field
	_, exists = ctx.GetFieldFloat64("nonexistent")
	if exists {
		t.Error("Expected nonexistent field to not exist")
	}

	t.Log("✅ Float64 field access working")
}

func TestGetFieldBool(t *testing.T) {
	jsonData := []byte(`{
		"available": true,
		"featured": false
	}`)

	ctx, err := NewDocumentContext("doc1", 1.0, jsonData)
	if err != nil {
		t.Fatalf("Failed to create context: %v", err)
	}

	// Test true boolean
	available, exists := ctx.GetFieldBool("available")
	if !exists {
		t.Error("Expected available field to exist")
	}
	if !available {
		t.Error("Expected available to be true")
	}

	// Test false boolean
	featured, exists := ctx.GetFieldBool("featured")
	if !exists {
		t.Error("Expected featured field to exist")
	}
	if featured {
		t.Error("Expected featured to be false")
	}

	// Test non-existent field
	_, exists = ctx.GetFieldBool("nonexistent")
	if exists {
		t.Error("Expected nonexistent field to not exist")
	}

	t.Log("✅ Bool field access working")
}

func TestNestedFieldAccess(t *testing.T) {
	jsonData := []byte(`{
		"metadata": {
			"category": "electronics",
			"vendor": {
				"name": "Apple",
				"country": "USA"
			}
		}
	}`)

	ctx, err := NewDocumentContext("doc1", 1.0, jsonData)
	if err != nil {
		t.Fatalf("Failed to create context: %v", err)
	}

	// Test one level deep
	category, exists := ctx.GetFieldString("metadata.category")
	if !exists {
		t.Error("Expected metadata.category to exist")
	}
	if category != "electronics" {
		t.Errorf("Expected category 'electronics', got '%s'", category)
	}

	// Test two levels deep
	vendorName, exists := ctx.GetFieldString("metadata.vendor.name")
	if !exists {
		t.Error("Expected metadata.vendor.name to exist")
	}
	if vendorName != "Apple" {
		t.Errorf("Expected vendor name 'Apple', got '%s'", vendorName)
	}

	t.Log("✅ Nested field access working")
}

func TestArrayFieldAccess(t *testing.T) {
	jsonData := []byte(`{
		"tags": ["new", "featured", "bestseller"],
		"prices": [99.99, 89.99, 79.99]
	}`)

	ctx, err := NewDocumentContext("doc1", 1.0, jsonData)
	if err != nil {
		t.Fatalf("Failed to create context: %v", err)
	}

	// Test array element access
	firstTag, exists := ctx.GetFieldString("tags[0]")
	if !exists {
		t.Error("Expected tags[0] to exist")
	}
	if firstTag != "new" {
		t.Errorf("Expected first tag 'new', got '%s'", firstTag)
	}

	// Test array element with index 2
	thirdTag, exists := ctx.GetFieldString("tags[2]")
	if !exists {
		t.Error("Expected tags[2] to exist")
	}
	if thirdTag != "bestseller" {
		t.Errorf("Expected third tag 'bestseller', got '%s'", thirdTag)
	}

	// Test numeric array
	firstPrice, exists := ctx.GetFieldFloat64("prices[0]")
	if !exists {
		t.Error("Expected prices[0] to exist")
	}
	if firstPrice != 99.99 {
		t.Errorf("Expected first price 99.99, got %f", firstPrice)
	}

	// Test out of bounds
	_, exists = ctx.GetFieldString("tags[10]")
	if exists {
		t.Error("Expected out of bounds access to fail")
	}

	t.Log("✅ Array field access working")
}

func TestHasField(t *testing.T) {
	jsonData := []byte(`{
		"title": "Test",
		"price": 99.99
	}`)

	ctx, err := NewDocumentContext("doc1", 1.0, jsonData)
	if err != nil {
		t.Fatalf("Failed to create context: %v", err)
	}

	// Test existing fields
	if !ctx.HasField("title") {
		t.Error("Expected title field to exist")
	}

	if !ctx.HasField("price") {
		t.Error("Expected price field to exist")
	}

	// Test non-existent field
	if ctx.HasField("nonexistent") {
		t.Error("Expected nonexistent field to not exist")
	}

	t.Log("✅ HasField working correctly")
}

func TestFieldAccessCount(t *testing.T) {
	jsonData := []byte(`{
		"title": "Test",
		"price": 99.99
	}`)

	ctx, err := NewDocumentContext("doc1", 1.0, jsonData)
	if err != nil {
		t.Fatalf("Failed to create context: %v", err)
	}

	if ctx.GetFieldAccessCount() != 0 {
		t.Errorf("Expected 0 accesses, got %d", ctx.GetFieldAccessCount())
	}

	// Access some fields
	ctx.GetFieldString("title")
	ctx.GetFieldFloat64("price")
	ctx.GetFieldString("nonexistent")

	if ctx.GetFieldAccessCount() != 3 {
		t.Errorf("Expected 3 accesses, got %d", ctx.GetFieldAccessCount())
	}

	t.Log("✅ Field access counting working")
}

func TestContextPool(t *testing.T) {
	pool := NewContextPool(3)
	defer pool.Close()

	jsonData1 := []byte(`{"title": "Doc 1"}`)
	jsonData2 := []byte(`{"title": "Doc 2"}`)

	// Get context from pool
	ctx1, err := pool.Get("doc1", 1.0, jsonData1)
	if err != nil {
		t.Fatalf("Failed to get context from pool: %v", err)
	}

	title1, _ := ctx1.GetFieldString("title")
	if title1 != "Doc 1" {
		t.Errorf("Expected title 'Doc 1', got '%s'", title1)
	}

	// Return to pool
	pool.Put(ctx1)

	// Get again (should reuse)
	ctx2, err := pool.Get("doc2", 2.0, jsonData2)
	if err != nil {
		t.Fatalf("Failed to get context from pool: %v", err)
	}

	title2, _ := ctx2.GetFieldString("title")
	if title2 != "Doc 2" {
		t.Errorf("Expected title 'Doc 2', got '%s'", title2)
	}

	// Verify context was reused (same pointer)
	if ctx1 != ctx2 {
		t.Log("Note: Context was not reused (pool may have been empty)")
	} else {
		t.Log("✅ Context pooling working - context was reused")
	}

	pool.Put(ctx2)

	t.Log("✅ Context pool working correctly")
}

func TestNewDocumentContextFromMap(t *testing.T) {
	data := map[string]interface{}{
		"title":    "Test Document",
		"price":    99.99,
		"quantity": 10,
		"available": true,
	}

	ctx := NewDocumentContextFromMap("doc1", 1.5, data)

	if ctx.GetDocumentID() != "doc1" {
		t.Errorf("Expected document ID 'doc1', got '%s'", ctx.GetDocumentID())
	}

	title, exists := ctx.GetFieldString("title")
	if !exists || title != "Test Document" {
		t.Errorf("Expected title 'Test Document', got '%s'", title)
	}

	price, exists := ctx.GetFieldFloat64("price")
	if !exists || price != 99.99 {
		t.Errorf("Expected price 99.99, got %f", price)
	}

	t.Log("✅ Context creation from map working")
}

func TestTypeConversion(t *testing.T) {
	jsonData := []byte(`{
		"int_field": 42,
		"float_field": 99.99
	}`)

	ctx, err := NewDocumentContext("doc1", 1.0, jsonData)
	if err != nil {
		t.Fatalf("Failed to create context: %v", err)
	}

	// Test int → float conversion
	floatVal, exists := ctx.GetFieldFloat64("int_field")
	if !exists {
		t.Error("Expected int_field to exist")
	}
	if floatVal != 42.0 {
		t.Errorf("Expected float value 42.0, got %f", floatVal)
	}

	// Test float → int conversion (truncation)
	intVal, exists := ctx.GetFieldInt64("float_field")
	if !exists {
		t.Error("Expected float_field to exist")
	}
	if intVal != 99 {
		t.Errorf("Expected int value 99, got %d", intVal)
	}

	t.Log("✅ Type conversion working correctly")
}

func BenchmarkFieldAccess(b *testing.B) {
	jsonData := []byte(`{
		"title": "Test Document",
		"price": 99.99,
		"quantity": 10,
		"available": true,
		"metadata": {
			"category": "electronics"
		}
	}`)

	ctx, _ := NewDocumentContext("doc1", 1.0, jsonData)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ctx.GetFieldString("title")
		ctx.GetFieldFloat64("price")
		ctx.GetFieldInt64("quantity")
		ctx.GetFieldBool("available")
		ctx.GetFieldString("metadata.category")
	}
}

func BenchmarkContextPool(b *testing.B) {
	pool := NewContextPool(10)
	defer pool.Close()

	jsonData := []byte(`{"title": "Test"}`)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ctx, _ := pool.Get("doc1", 1.0, jsonData)
		ctx.GetFieldString("title")
		pool.Put(ctx)
	}
}
