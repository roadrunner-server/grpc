<?php
# Generated by the protocol buffer compiler (roadrunner-server/grpc). DO NOT EDIT!
# source: import/service.proto

namespace Import;

use Spiral\RoadRunner\GRPC;

interface ServiceInterface extends GRPC\ServiceInterface
{
    // GRPC specific service name.
    public const NAME = "import.Service";

    /**
    * @param GRPC\ContextInterface $ctx
    * @param Message $in
    * @return Message
    *
    * @throws GRPC\Exception\InvokeException
    */
    public function SimpleMethod(GRPC\ContextInterface $ctx, Message $in): Message;

    /**
    * @param GRPC\ContextInterface $ctx
    * @param \Import\Sub\Message $in
    * @return \Import\Sub\Message
    *
    * @throws GRPC\Exception\InvokeException
    */
    public function ImportMethod(GRPC\ContextInterface $ctx, \Import\Sub\Message $in): \Import\Sub\Message;
}
