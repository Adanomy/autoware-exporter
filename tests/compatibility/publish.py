#!/usr/bin/env python3
"""Publish synthetic fixtures only inside the compatibility test container."""

import json
from pathlib import Path
import sys

import rclpy
from rclpy.qos import DurabilityPolicy, QoSProfile, ReliabilityPolicy
from rosidl_runtime_py.set_message import set_message_fields
from rosidl_runtime_py.utilities import get_message


def constants(value):
    if isinstance(value, dict):
        if set(value) == {"$constant"}:
            message, name = value["$constant"].rsplit(".", 1)
            return getattr(get_message(message), name)
        if set(value) == {"$byte"}:
            return bytes([value["$byte"]])
        return {key: constants(item) for key, item in value.items()}
    if isinstance(value, list):
        return [constants(item) for item in value]
    return value


def main():
    # Go supplies the actual production source list; stale fixture coverage fails.
    sources = json.load(sys.stdin)
    fixtures = json.loads(Path(__file__).with_name("messages.json").read_text())
    if {source["Key"] for source in sources} != set(fixtures):
        raise ValueError("Compatibility fixtures must cover every configured source")
    rclpy.init(domain_id=200)
    node = rclpy.create_node("autoware_exporter_compatibility_publisher")
    try:
        publishers = []
        for source in sources:
            message_type = get_message(source["Type"])
            message = message_type()
            set_message_fields(message, constants(fixtures[source["Key"]]))
            qos = QoSProfile(depth=1, reliability=ReliabilityPolicy.RELIABLE,
                             durability=DurabilityPolicy.TRANSIENT_LOCAL if source["Latched"] else DurabilityPolicy.VOLATILE)
            publisher = node.create_publisher(message_type, "/compatibility" + source["Topic"], qos)
            publishers.append((publisher, message))
        print("READY", flush=True)

        def publish():
            for publisher, message in publishers:
                publisher.publish(message)

        node.create_timer(0.1, publish)
        rclpy.spin(node)
    finally:
        node.destroy_node()
        rclpy.shutdown()


if __name__ == "__main__":
    main()
