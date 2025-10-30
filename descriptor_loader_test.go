package grpc

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
)

func TestLoadDescriptorSets(t *testing.T) {
	tests := []struct {
		name           string
		descriptorSets []string
		setup          func(t *testing.T) []string
		expectedError  bool
		verify         func(t *testing.T, p *Plugin)
	}{
		{
			name:           "no descriptor sets",
			descriptorSets: []string{},
			setup:          nil,
			expectedError:  false,
			verify: func(t *testing.T, p *Plugin) {
				_, err := protoregistry.GlobalFiles.FindDescriptorByName("test.Test")
				assert.Error(t, err, "no descriptors should be registered")
			},
		},
		{
			name: "real test.pb descriptor",
			setup: func(t *testing.T) []string {
				testPbPath := filepath.Join("tests", "proto", "test", "test.pb")

				require.FileExists(t, testPbPath, "test.pb should exist in tests/proto/test/")

				return []string{testPbPath}
			},
			expectedError: false,
			verify: func(t *testing.T, p *Plugin) {
				desc, err := protoregistry.GlobalFiles.FindDescriptorByName("test.Test")
				require.NoError(t, err, "test.Test service should be registered")

				serviceDesc, ok := desc.(protoreflect.ServiceDescriptor)
				require.True(t, ok, "descriptor should be a ServiceDescriptor")

				// Verify service name
				assert.Equal(t, "Test", string(serviceDesc.Name()))
				assert.Equal(t, "test", string(serviceDesc.ParentFile().Package()))

				// Verify methods exist
				methods := serviceDesc.Methods()
				assert.Equal(t, 5, methods.Len(), "Test service should have 5 methods")

				// Check individual methods
				echoMethod := methods.ByName("Echo")
				require.NotNil(t, echoMethod, "Echo method should exist")
				assert.Equal(t, "Echo", string(echoMethod.Name()))

				throwMethod := methods.ByName("Throw")
				require.NotNil(t, throwMethod, "Throw method should exist")

				dieMethod := methods.ByName("Die")
				require.NotNil(t, dieMethod, "Die method should exist")

				infoMethod := methods.ByName("Info")
				require.NotNil(t, infoMethod, "Info method should exist")

				pingMethod := methods.ByName("Ping")
				require.NotNil(t, pingMethod, "Ping method should exist")

				// Verify message types
				_, err = protoregistry.GlobalFiles.FindDescriptorByName("test.Message")
				assert.NoError(t, err, "test.Message should be registered")

				_, err = protoregistry.GlobalFiles.FindDescriptorByName("test.EmptyMessage")
				assert.NoError(t, err, "test.EmptyMessage should be registered")

				_, err = protoregistry.GlobalFiles.FindDescriptorByName("test.DetailsMessageForException")
				assert.NoError(t, err, "test.DetailsMessageForException should be registered")
			},
		},
		{
			name: "valid descriptor set",
			setup: func(t *testing.T) []string {
				tempFile := filepath.Join(t.TempDir(), "test.desc")

				fd := &descriptorpb.FileDescriptorProto{
					Name:    proto.String("minimal.proto"),
					Package: proto.String("minimal"),
					MessageType: []*descriptorpb.DescriptorProto{{
						Name: proto.String("TestMessage"),
					}},
					Service: []*descriptorpb.ServiceDescriptorProto{{
						Name: proto.String("MinimalService"),
					}},
				}

				fds := &descriptorpb.FileDescriptorSet{
					File: []*descriptorpb.FileDescriptorProto{fd},
				}

				data, err := proto.Marshal(fds)
				require.NoError(t, err)
				err = os.WriteFile(tempFile, data, 0o644)
				require.NoError(t, err)

				return []string{tempFile}
			},
			expectedError: false,
			verify: func(t *testing.T, p *Plugin) {
				_, err := protoregistry.GlobalFiles.FindDescriptorByName("minimal.MinimalService")
				assert.NoError(t, err, "minimal.MinimalService should be registered")
			},
		},
		{
			name: "invalid descriptor file",
			setup: func(t *testing.T) []string {
				tempFile := filepath.Join(t.TempDir(), "invalid.desc")
				err := os.WriteFile(tempFile, []byte("invalid data"), 0o644)
				require.NoError(t, err)
				return []string{tempFile}
			},
			expectedError: false,
			verify: func(t *testing.T, p *Plugin) {
				_, err := protoregistry.GlobalFiles.FindDescriptorByName("test.Test")
				assert.Error(t, err, "no descriptors should be registered for invalid files")
			},
		},
		{
			name: "non-existent file",
			setup: func(t *testing.T) []string {
				return []string{filepath.Join(t.TempDir(), "nonexistent.desc")}
			},
			expectedError: false,
			verify: func(t *testing.T, p *Plugin) {
				_, err := protoregistry.GlobalFiles.FindDescriptorByName("test.Test")
				assert.Error(t, err, "no descriptors should be registered for non-existent files")
			},
		},
		{
			name: "duplicate registration",
			setup: func(t *testing.T) []string {
				testPbPath := filepath.Join("tests", "proto", "test", "test.pb")
				require.FileExists(t, testPbPath)
				return []string{testPbPath, testPbPath} // Same file twice
			},
			expectedError: false,
			verify: func(t *testing.T, p *Plugin) {
				_, err := protoregistry.GlobalFiles.FindDescriptorByName("test.Test")
				assert.NoError(t, err, "test.Test service should be registered")

				var fileCount int
				protoregistry.GlobalFiles.RangeFiles(func(fd protoreflect.FileDescriptor) bool {
					if fd.Path() == "test.proto" {
						fileCount++
					}
					return true
				})
				assert.Equal(t, 1, fileCount, "file should be registered only once")
			},
		},
		{
			name: "multiple different descriptor sets",
			setup: func(t *testing.T) []string {
				testPbPath := filepath.Join("tests", "proto", "test", "test.pb")
				require.FileExists(t, testPbPath)

				tempFile := filepath.Join(t.TempDir(), "another.desc")
				fd := &descriptorpb.FileDescriptorProto{
					Name:    proto.String("another.proto"),
					Package: proto.String("another"),
					Service: []*descriptorpb.ServiceDescriptorProto{{
						Name: proto.String("AnotherService"),
					}},
				}
				fds := &descriptorpb.FileDescriptorSet{
					File: []*descriptorpb.FileDescriptorProto{fd},
				}
				data, _ := proto.Marshal(fds)
				os.WriteFile(tempFile, data, 0o644)

				return []string{testPbPath, tempFile}
			},
			expectedError: false,
			verify: func(t *testing.T, p *Plugin) {
				_, err1 := protoregistry.GlobalFiles.FindDescriptorByName("test.Test")
				assert.NoError(t, err1, "test.Test should be registered")

				_, err2 := protoregistry.GlobalFiles.FindDescriptorByName("another.AnotherService")
				assert.NoError(t, err2, "another.AnotherService should be registered")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			protoregistry.GlobalFiles = new(protoregistry.Files)

			var descriptorSets []string
			if tt.setup != nil {
				descriptorSets = tt.setup(t)
			} else {
				descriptorSets = tt.descriptorSets
			}

			p := &Plugin{
				config: &Config{
					Reflection: &ReflectionConfig{
						Enabled:        true,
						DescriptorSets: descriptorSets,
					},
				},
				log: zap.NewNop(),
			}

			err := p.loadDescriptorSets()
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			if tt.verify != nil {
				tt.verify(t, p)
			}
		})
	}
}
