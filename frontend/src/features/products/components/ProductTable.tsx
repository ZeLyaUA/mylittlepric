"use client";

import { useState } from "react";
import { ExternalLink } from "lucide-react";
import { Product, ProductCard as ProductCardType } from "@/shared/types";
import { ProductDrawer } from "./ProductDrawer";

interface ProductTableProps {
  products: (Product | ProductCardType)[];
  description?: string;
}

export function ProductTable({ products, description }: ProductTableProps) {
  const [selectedToken, setSelectedToken] = useState<string | null>(null);
  const [showAll, setShowAll] = useState(false);

  // Helper to check if product is of type Product
  const isProduct = (p: Product | ProductCardType): p is Product => 'thumbnail' in p;

  // Limit to 4 products initially
  const MAX_VISIBLE = 4;
  const hasMore = products.length > MAX_VISIBLE;
  const visibleProducts = showAll ? products : products.slice(0, MAX_VISIBLE);

  return (
    <>
      <div className="w-full space-y-4">
        {/* AI Description about the product */}
        {description && (
          <div className="bg-gradient-to-r from-primary/5 to-secondary/5 border border-primary/20 rounded-lg p-3 md:p-4 shadow-sm">
            <div className="flex items-start gap-2">
              <div className="flex-shrink-0 w-4 h-4 md:w-5 md:h-5 mt-0.5">
                <svg className="w-full h-full text-primary/70" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                </svg>
              </div>
              <div className="flex-1">
                <p className="text-xs md:text-sm text-foreground/90 leading-relaxed font-medium">
                  {description}
                </p>
              </div>
            </div>
          </div>
        )}

        {/* Desktop Table View - hidden on mobile */}
        <div className="hidden md:block bg-card border border-border rounded-lg overflow-hidden">
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead>
                <tr className="border-b border-border bg-muted/30">
                  <th className="text-left py-3 px-4 text-xs font-semibold text-muted-foreground uppercase tracking-wider w-20">
                    Image
                  </th>
                  <th className="text-left py-3 px-4 text-xs font-semibold text-muted-foreground uppercase tracking-wider">
                    Product
                  </th>
                  <th className="text-right py-3 px-4 text-xs font-semibold text-muted-foreground uppercase tracking-wider w-32">
                    Action
                  </th>
                </tr>
              </thead>
              <tbody className="divide-y divide-border/50">
                {visibleProducts.map((product, index) => {
                  const image = isProduct(product) ? product.thumbnail : product.image;
                  const name = isProduct(product) ? product.title : product.name;
                  const badge = isProduct(product) ? (product.rating ? `⭐ ${product.rating}` : undefined) : product.badge;

                  return (
                    <tr
                      key={index}
                      className="hover:bg-muted/20 transition-colors duration-150 group"
                    >
                      {/* Mini Image */}
                      <td className="py-3 px-4">
                        <div
                          className="relative w-16 h-16 rounded-md overflow-hidden bg-muted cursor-pointer"
                          onClick={() => product.page_token && setSelectedToken(product.page_token)}
                        >
                          <img
                            src={image}
                            alt={name}
                            className="w-full h-full object-cover group-hover:scale-105 transition-transform duration-300"
                          />
                          {badge && (
                            <div className="absolute bottom-0 left-0 right-0 bg-black/70 backdrop-blur-sm text-white text-[9px] px-1 py-0.5 text-center">
                              {badge}
                            </div>
                          )}
                        </div>
                      </td>

                      {/* Product Name and Price */}
                      <td className="py-3 px-4">
                        <div className="space-y-1">
                          <h3
                            className="font-medium text-sm text-foreground line-clamp-2 leading-snug cursor-pointer hover:text-primary transition-colors"
                            onClick={() => product.page_token && setSelectedToken(product.page_token)}
                          >
                            {name}
                          </h3>
                          <div className="flex items-baseline gap-2">
                            <span className="text-base font-bold bg-gradient-to-r from-primary to-primary/80 bg-clip-text text-transparent">
                              {product.price}
                            </span>
                          </div>
                        </div>
                      </td>

                      {/* See All Sellers Button */}
                      <td className="py-3 px-4 text-right">
                        <button
                          onClick={() => product.page_token && setSelectedToken(product.page_token)}
                          className="inline-flex items-center justify-center gap-1.5 text-xs font-medium text-primary-foreground bg-gradient-to-r from-primary to-primary/90 hover:from-primary hover:to-primary px-4 py-2 rounded-md transition-all duration-300 hover:shadow-lg hover:shadow-primary/20 hover:scale-[1.02] relative overflow-hidden group/btn"
                        >
                          <div className="absolute inset-0 bg-gradient-to-r from-transparent via-white/20 to-transparent opacity-0 group-hover/btn:opacity-100 group-hover/btn:animate-shimmer" />
                          <span className="relative">See All Sellers</span>
                          <ExternalLink className="w-3 h-3 group-hover/btn:translate-x-0.5 transition-transform duration-300 relative" />
                        </button>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>

          {/* Desktop Show More/Less Buttons */}
          {hasMore && !showAll && (
            <div className="relative">
              <div className="absolute inset-x-0 -top-20 h-20 bg-gradient-to-t from-card to-transparent pointer-events-none" />
              <div className="p-4 bg-card border-t border-border flex justify-center">
                <button
                  onClick={() => setShowAll(true)}
                  className="inline-flex items-center gap-2 px-6 py-3 bg-gradient-to-r from-primary to-primary/90 hover:from-primary hover:to-primary text-primary-foreground font-medium rounded-lg transition-all duration-300 hover:shadow-lg hover:shadow-primary/20 hover:scale-[1.02] group"
                >
                  <span>Просмотреть все предложения ({products.length})</span>
                  <svg className="w-4 h-4 group-hover:translate-y-0.5 transition-transform" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
                  </svg>
                </button>
              </div>
            </div>
          )}

          {hasMore && showAll && (
            <div className="p-4 bg-card border-t border-border flex justify-center">
              <button
                onClick={() => setShowAll(false)}
                className="inline-flex items-center gap-2 px-6 py-3 bg-secondary hover:bg-secondary/80 text-secondary-foreground font-medium rounded-lg transition-all duration-200 group"
              >
                <span>Свернуть</span>
                <svg className="w-4 h-4 group-hover:-translate-y-0.5 transition-transform" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 15l7-7 7 7" />
                </svg>
              </button>
            </div>
          )}
        </div>

        {/* Mobile Card View - shown only on mobile */}
        <div className="md:hidden space-y-3">
          {visibleProducts.map((product, index) => {
            const image = isProduct(product) ? product.thumbnail : product.image;
            const name = isProduct(product) ? product.title : product.name;
            const badge = isProduct(product) ? (product.rating ? `⭐ ${product.rating}` : undefined) : product.badge;

            return (
              <div
                key={index}
                className="bg-card border border-border rounded-lg overflow-hidden hover:shadow-lg transition-shadow duration-200"
                onClick={() => product.page_token && setSelectedToken(product.page_token)}
              >
                <div className="flex gap-3 p-3">
                  {/* Product Image */}
                  <div className="relative flex-shrink-0 w-20 h-20 rounded-md overflow-hidden bg-muted">
                    <img
                      src={image}
                      alt={name}
                      className="w-full h-full object-cover"
                    />
                    {badge && (
                      <div className="absolute bottom-0 left-0 right-0 bg-black/70 backdrop-blur-sm text-white text-[9px] px-1 py-0.5 text-center">
                        {badge}
                      </div>
                    )}
                  </div>

                  {/* Product Info */}
                  <div className="flex-1 min-w-0 flex flex-col justify-between">
                    <h3 className="font-medium text-sm text-foreground line-clamp-2 leading-snug mb-1">
                      {name}
                    </h3>

                    <div className="flex items-center justify-between gap-2">
                      <span className="text-base font-bold bg-gradient-to-r from-primary to-primary/80 bg-clip-text text-transparent">
                        {product.price}
                      </span>

                      <button
                        onClick={(e) => {
                          e.stopPropagation();
                          product.page_token && setSelectedToken(product.page_token);
                        }}
                        className="inline-flex items-center gap-1 text-xs font-medium text-primary-foreground bg-primary hover:bg-primary/90 px-3 py-1.5 rounded-md transition-colors"
                      >
                        <span>View</span>
                        <ExternalLink className="w-3 h-3" />
                      </button>
                    </div>
                  </div>
                </div>
              </div>
            );
          })}

          {/* Mobile Show More/Less Buttons */}
          {hasMore && !showAll && (
            <div className="relative">
              <div className="absolute inset-x-0 -top-12 h-12 bg-gradient-to-t from-background to-transparent pointer-events-none" />
              <button
                onClick={() => setShowAll(true)}
                className="w-full flex items-center justify-center gap-2 px-4 py-3 bg-gradient-to-r from-primary to-primary/90 text-primary-foreground font-medium rounded-lg transition-all duration-300 hover:shadow-lg hover:shadow-primary/20 group"
              >
                <span className="text-sm">Показать все ({products.length})</span>
                <svg className="w-4 h-4 group-hover:translate-y-0.5 transition-transform" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
                </svg>
              </button>
            </div>
          )}

          {hasMore && showAll && (
            <button
              onClick={() => setShowAll(false)}
              className="w-full flex items-center justify-center gap-2 px-4 py-3 bg-secondary hover:bg-secondary/80 text-secondary-foreground font-medium rounded-lg transition-all duration-200 group"
            >
              <span className="text-sm">Свернуть</span>
              <svg className="w-4 h-4 group-hover:-translate-y-0.5 transition-transform" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 15l7-7 7 7" />
              </svg>
            </button>
          )}
        </div>
      </div>

      {selectedToken && (
        <ProductDrawer
          pageToken={selectedToken}
          onClose={() => setSelectedToken(null)}
        />
      )}
    </>
  );
}
