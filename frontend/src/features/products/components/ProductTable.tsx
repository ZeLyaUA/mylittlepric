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
          <div className="bg-gradient-to-r from-primary/5 to-secondary/5 border border-primary/20 rounded-lg p-4 shadow-sm">
            <div className="flex items-start gap-2">
              <div className="flex-shrink-0 w-5 h-5 mt-0.5">
                <svg className="w-5 h-5 text-primary/70" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                </svg>
              </div>
              <div className="flex-1">
                <p className="text-sm text-foreground/90 leading-relaxed font-medium">
                  {description}
                </p>
              </div>
            </div>
          </div>
        )}

        {/* Products Table */}
        <div className="bg-card border border-border rounded-lg overflow-hidden">
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
                          {/* Shimmer effect */}
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

          {/* Show More Button - only if there are more than MAX_VISIBLE products */}
          {hasMore && !showAll && (
            <div className="relative">
              {/* Fade overlay */}
              <div className="absolute inset-x-0 -top-20 h-20 bg-gradient-to-t from-card to-transparent pointer-events-none" />

              {/* Button */}
              <div className="p-4 bg-card border-t border-border flex justify-center">
                <button
                  onClick={() => setShowAll(true)}
                  className="inline-flex items-center gap-2 px-6 py-3 bg-gradient-to-r from-primary to-primary/90 hover:from-primary hover:to-primary text-primary-foreground font-medium rounded-lg transition-all duration-300 hover:shadow-lg hover:shadow-primary/20 hover:scale-[1.02] group"
                >
                  <span>Просмотреть все предложения ({products.length})</span>
                  <svg
                    className="w-4 h-4 group-hover:translate-y-0.5 transition-transform"
                    fill="none"
                    stroke="currentColor"
                    viewBox="0 0 24 24"
                  >
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
                  </svg>
                </button>
              </div>
            </div>
          )}

          {/* Show Less Button - when expanded */}
          {hasMore && showAll && (
            <div className="p-4 bg-card border-t border-border flex justify-center">
              <button
                onClick={() => setShowAll(false)}
                className="inline-flex items-center gap-2 px-6 py-3 bg-secondary hover:bg-secondary/80 text-secondary-foreground font-medium rounded-lg transition-all duration-200 group"
              >
                <span>Свернуть</span>
                <svg
                  className="w-4 h-4 group-hover:-translate-y-0.5 transition-transform"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                >
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 15l7-7 7 7" />
                </svg>
              </button>
            </div>
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
