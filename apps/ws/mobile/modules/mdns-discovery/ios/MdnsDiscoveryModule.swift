import ExpoModulesCore
import Network

public class MdnsDiscoveryModule: Module {
  private var browser: NWBrowser?
  private var resolvers: [NetServiceResolver] = []

  public func definition() -> ModuleDefinition {
    Name("MdnsDiscovery")

    Events("onServiceFound", "onScanError")

    Function("startScan") { (serviceType: String) in
      self.stopAll()

      let descriptor = NWBrowser.Descriptor.bonjour(type: serviceType, domain: "local.")
      let params = NWParameters()
      params.includePeerToPeer = true

      let browser = NWBrowser(for: descriptor, using: params)

      browser.stateUpdateHandler = { [weak self] state in
        switch state {
        case .ready:
          break
        case .failed(let error):
          DispatchQueue.main.async {
            self?.sendEvent("onScanError", ["message": error.localizedDescription])
          }
        default:
          break
        }
      }

      browser.browseResultsChangedHandler = { [weak self] results, changes in
        for change in changes {
          switch change {
          case .added(let result):
            if case let .service(name, type, domain, _) = result.endpoint {
              self?.resolveWithNetService(name: name, type: type, domain: domain)
            }
          default:
            break
          }
        }
      }

      browser.start(queue: .main)
      self.browser = browser
    }

    Function("stopScan") {
      self.stopAll()
    }
  }

  private func stopAll() {
    browser?.cancel()
    browser = nil
    for resolver in resolvers {
      resolver.stop()
    }
    resolvers.removeAll()
  }

  private func resolveWithNetService(name: String, type: String, domain: String) {
    let resolver = NetServiceResolver(name: name, type: type, domain: domain) { [weak self] service in
      guard let self = self else { return }

      var addresses: [String] = []
      if let addrs = service.addresses {
        for addrData in addrs {
          if let str = self.parseAddress(addrData) {
            addresses.append(str)
          }
        }
      }

      // Parse TXT record
      var txt: [String: String] = [:]
      if let txtData = service.txtRecordData() {
        let dict = NetService.dictionary(fromTXTRecord: txtData)
        for (key, value) in dict {
          txt[key] = String(data: value, encoding: .utf8) ?? ""
        }
      }

      let host = addresses.first(where: { $0.contains(".") && !$0.contains(":") }) ?? service.hostName ?? name

      self.sendEvent("onServiceFound", [
        "name": name,
        "host": host.hasSuffix(".") ? String(host.dropLast()) : host,
        "port": service.port,
        "addresses": addresses,
        "txt": txt,
      ])
    }

    resolvers.append(resolver)
    resolver.start()
  }

  private func parseAddress(_ data: Data) -> String? {
    var storage = sockaddr_storage()
    guard data.count >= MemoryLayout<sockaddr>.size else { return nil }
    data.withUnsafeBytes { ptr in
      _ = withUnsafeMutableBytes(of: &storage) { dest in
        dest.copyMemory(from: UnsafeRawBufferPointer(rebasing: ptr.prefix(min(data.count, dest.count))))
      }
    }

    if storage.ss_family == UInt8(AF_INET) {
      var addr = withUnsafeBytes(of: &storage) { ptr in
        ptr.load(as: sockaddr_in.self)
      }
      var buf = [CChar](repeating: 0, count: Int(INET_ADDRSTRLEN))
      inet_ntop(AF_INET, &addr.sin_addr, &buf, socklen_t(INET_ADDRSTRLEN))
      return String(cString: buf)
    } else if storage.ss_family == UInt8(AF_INET6) {
      var addr = withUnsafeBytes(of: &storage) { ptr in
        ptr.load(as: sockaddr_in6.self)
      }
      var buf = [CChar](repeating: 0, count: Int(INET6_ADDRSTRLEN))
      inet_ntop(AF_INET6, &addr.sin6_addr, &buf, socklen_t(INET6_ADDRSTRLEN))
      return String(cString: buf)
    }
    return nil
  }
}

// Helper class that wraps NSNetService resolution
class NetServiceResolver: NSObject, NetServiceDelegate {
  private let service: NetService
  private let completion: (NetService) -> Void

  init(name: String, type: String, domain: String, completion: @escaping (NetService) -> Void) {
    self.service = NetService(domain: domain, type: type, name: name)
    self.completion = completion
    super.init()
    self.service.delegate = self
  }

  func start() {
    service.resolve(withTimeout: 5.0)
  }

  func stop() {
    service.stop()
  }

  func netServiceDidResolveAddress(_ sender: NetService) {
    completion(sender)
  }

  func netService(_ sender: NetService, didNotResolve errorDict: [String: NSNumber]) {
    // Resolution failed — ignore
  }
}
