<?php
/**
 * Sample GRPC PHP server.
 */

use Spiral\RoadRunner\GRPC\ContextInterface;
use Service\EchoInterface;
use Service\Message;

class EchoServiceException implements EchoInterface
{
    public function Ping(ContextInterface $ctx, Message $in): Message
    {
        throw \Spiral\RoadRunner\GRPC\Exception\InvokeException::create(
            'FOOOOOOOOOOOO',
                \Spiral\RoadRunner\GRPC\StatusCode::INTERNAL
         );
        return $out->setMsg(strtoupper($in->getMsg()));
    }
}
