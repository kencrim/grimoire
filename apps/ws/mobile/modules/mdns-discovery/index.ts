import { requireNativeModule, EventEmitter } from 'expo-modules-core';

const MdnsDiscovery = requireNativeModule('MdnsDiscovery');
const emitter = new EventEmitter(MdnsDiscovery);

export interface MdnsService {
  name: string;
  host: string;
  port: number;
  addresses: string[];
  txt: Record<string, string>;
}

export function startScan(serviceType: string): void {
  MdnsDiscovery.startScan(serviceType);
}

export function stopScan(): void {
  MdnsDiscovery.stopScan();
}

export function onServiceFound(callback: (service: MdnsService) => void): { remove: () => void } {
  return emitter.addListener('onServiceFound', callback);
}

export function onScanError(callback: (error: { message: string }) => void): { remove: () => void } {
  return emitter.addListener('onScanError', callback);
}
