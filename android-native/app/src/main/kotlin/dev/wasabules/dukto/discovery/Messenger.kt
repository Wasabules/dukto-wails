package dev.wasabules.dukto.discovery

/**
 * UDP messenger: HELLO_BROADCAST / HELLO_UNICAST / GOODBYE.
 *
 * Reference: wails/internal/discovery/messenger.go
 *
 * Notes specific to Android:
 *  - WifiManager.MulticastLock is required to receive UDP broadcast on
 *    most Android devices. Acquire it at start, release at shutdown.
 *  - For simultaneous multi-app coexistence on the same device the
 *    socket needs SO_REUSEADDR + SO_REUSEPORT.
 *
 * TODO: implement with java.net.DatagramSocket. Self-echo suppression,
 * per-source HELLO cooldown, and the broadcast-storm guard mirror the
 * Go side.
 */
class Messenger
