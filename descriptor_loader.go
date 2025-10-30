package grpc

import (
	"os"

	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
)

func (p *Plugin) loadDescriptorSets() error {
	for _, descriptorFile := range p.config.DescriptorSets {
		if descriptorFile == "" {
			continue
		}

		data, err := os.ReadFile(descriptorFile)
		if err != nil {
			p.log.Warn("failed to read descriptor set",
				zap.String("file", descriptorFile),
				zap.Error(err))
			continue
		}

		var fds descriptorpb.FileDescriptorSet
		if err := proto.Unmarshal(data, &fds); err != nil {
			p.log.Warn("failed to unmarshal descriptor set",
				zap.String("file", descriptorFile),
				zap.Error(err))
			continue
		}

		files, err := protodesc.NewFiles(&fds)
		if err != nil {
			p.log.Warn("failed to create files from descriptor set",
				zap.String("file", descriptorFile),
				zap.Error(err))
			continue
		}

		var registeredCount int
		files.RangeFiles(func(fd protoreflect.FileDescriptor) bool {

			if _, err := protoregistry.GlobalFiles.FindFileByPath(fd.Path()); err == nil {
				return true
			}

			if err := protoregistry.GlobalFiles.RegisterFile(fd); err != nil {
				p.log.Warn("failed to register file descriptor",
					zap.String("path", string(fd.Path())),
					zap.Error(err))
			} else {
				registeredCount++
			}
			return true
		})

		p.log.Info("descriptor set loaded",
			zap.String("file", descriptorFile),
			zap.Int("files_registered", registeredCount))
	}

	return nil
}
