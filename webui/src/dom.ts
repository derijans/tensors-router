type ElementConstructor<T extends Element> = {
  new (): T;
};

export function getRequiredElement<T extends Element>(id: string, constructor: ElementConstructor<T>): T {
  const element = document.getElementById(id);
  if (!(element instanceof constructor)) {
    throw new Error(`Expected #${id} to be ${constructor.name}`);
  }
  return element;
}

export function elementTarget(event: Event): HTMLElement | null {
  return event.target instanceof HTMLElement ? event.target : null;
}

export function closestElement<T extends Element>(target: EventTarget | null, selector: string, constructor: ElementConstructor<T>): T | null {
  if (!(target instanceof Element)) {
    return null;
  }
  const element = target.closest(selector);
  return element instanceof constructor ? element : null;
}

export function queryElements<T extends Element>(selector: string, constructor: ElementConstructor<T>): T[] {
  return Array.from(document.querySelectorAll(selector)).filter((element): element is T => element instanceof constructor);
}
