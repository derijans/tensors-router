import { describe, expect, it } from "vitest";
import { SafeHTML, emptyHTML, html, listOrFallback, setHTML } from "../safe-html";

const hostile = `"><img src=x onerror=alert(1)>'\`&`;

describe("html template", () => {
  it("escapes interpolated text so it cannot open a tag", () => {
    expect(SafeHTML.render(html`<span>${hostile}</span>`)).toBe(
      "<span>&quot;&gt;&lt;img src=x onerror=alert(1)&gt;&#39;&#96;&amp;</span>"
    );
  });

  it("escapes interpolated attribute values so they cannot close the quote", () => {
    expect(SafeHTML.render(html`<a title="${hostile}">x</a>`)).toBe(
      `<a title="&quot;&gt;&lt;img src=x onerror=alert(1)&gt;&#39;&#96;&amp;">x</a>`
    );
  });

  it("keeps nested templates as markup while still escaping their values", () => {
    const inner = html`<b>${"<i>"}</b>`;
    expect(SafeHTML.render(html`<p>${inner}</p>`)).toBe("<p><b>&lt;i&gt;</b></p>");
  });

  it("renders arrays in order and treats absent values as empty", () => {
    const rows = ["a", "<b>"].map(value => html`<li>${value}</li>`);
    expect(SafeHTML.render(html`<ul>${rows}${null}${undefined}</ul>`)).toBe("<ul><li>a</li><li>&lt;b&gt;</li></ul>");
  });

  it("renders numbers and booleans as text", () => {
    expect(SafeHTML.render(html`${3}/${false}`)).toBe("3/false");
  });
});

describe("listOrFallback", () => {
  it("uses the fallback only when there are no items", () => {
    const fallback = html`<p>none</p>`;
    expect(SafeHTML.render(listOrFallback([], fallback))).toBe("<p>none</p>");
    expect(SafeHTML.render(listOrFallback([html`<p>one</p>`], fallback))).toBe("<p>one</p>");
  });
});

describe("setHTML", () => {
  it("writes the rendered markup and nothing else", () => {
    const element = {innerHTML: "stale"} as unknown as Element;
    setHTML(element, emptyHTML);
    expect(element.innerHTML).toBe("");
    setHTML(element, html`<em>${"<x>"}</em>`);
    expect(element.innerHTML).toBe("<em>&lt;x&gt;</em>");
  });
});
