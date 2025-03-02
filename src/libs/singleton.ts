export function singleton<T>(name: string, getValue: () => T): T {
  const thusly = globalThis as { qs_singletons?: { [key: string]: T } };
  thusly.qs_singletons ??= {};
  thusly.qs_singletons[name] ??= getValue();
  return thusly.qs_singletons![name];
}
