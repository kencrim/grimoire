// React Native's FormData polyfill accepts { uri, name, type } objects
// for file uploads. This augmentation makes TypeScript aware of that
// so we don't need typecasts when appending file-like objects.

interface ReactNativeFormDataFile {
  uri: string;
  name: string;
  type: string;
}

interface FormData {
  append(name: string, value: ReactNativeFormDataFile, fileName?: string): void;
}
