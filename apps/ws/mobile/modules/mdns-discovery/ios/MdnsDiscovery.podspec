Pod::Spec.new do |s|
  s.name           = 'MdnsDiscovery'
  s.version        = '0.1.0'
  s.summary        = 'mDNS/Bonjour service discovery for Grimoire'
  s.homepage       = 'https://github.com/kencrim/grimoire'
  s.license        = 'MIT'
  s.author         = 'Ken Crimmins'
  s.source         = { git: '' }
  s.platform       = :ios, '15.0'
  s.swift_version  = '5.9'
  s.source_files   = '**/*.swift'

  s.dependency 'ExpoModulesCore'
end
