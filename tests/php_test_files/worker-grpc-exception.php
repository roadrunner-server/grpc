<?php

/**
 * Sample GRPC PHP server.
 */

use Service\EchoInterface;
use Spiral\RoadRunner\GRPC\Server;
use Spiral\RoadRunner\Worker;

ini_set('display_errors', 'stderr');
require __DIR__ . '/vendor/autoload.php';

$server = new Server();

$server->registerService(EchoInterface::class, new EchoServiceException());

$server->serve(Worker::create());
