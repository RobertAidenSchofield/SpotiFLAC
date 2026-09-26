import { useState, useEffect, useCallback } from 'react';
import {
  Sparkles,
  RefreshCw,
  Disc,
  Music2,
  Play,
  Flame,
  Radio,
  User,
  Loader2,
} from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Card } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import {
  GetSpotifyHomeFeed,
  GetSpotifyCategoryFeed,
} from '../../wailsjs/go/main/App';
import type { backend } from '../../wailsjs/go/models';

interface HomeScreenProps {
  onSelectItem: (url: string) => void;
  isLoadingItem?: boolean;
}

const CATEGORIES = [
  { id: 'all', label: 'All' },
  { id: 'new-releases', label: 'New Releases' },
  { id: 'trending', label: 'Trending Albums' },
  { id: 'playlists', label: 'Featured Playlists' },
  { id: 'pop', label: 'Pop' },
  { id: 'hip-hop', label: 'Hip-Hop' },
  { id: 'rock', label: 'Rock' },
  { id: 'electronic', label: 'Electronic' },
  { id: 'r&b', label: 'R&B / Soul' },
  { id: 'indie', label: 'Indie' },
  { id: 'latin', label: 'Latin' },
];

export function HomeScreen({ onSelectItem, isLoadingItem }: HomeScreenProps) {
  const [selectedItemUrl, setSelectedItemUrl] = useState<string | null>(null);
  const [feed, setFeed] = useState<backend.SpotifyHomeFeedResponse | null>(
    null,
  );
  const [categoryItems, setCategoryItems] = useState<
    backend.SpotifyHomeItem[] | null
  >(null);
  const [selectedCategory, setSelectedCategory] = useState('all');
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const loadFeed = useCallback(async (forceRefresh = false) => {
    try {
      if (forceRefresh) {
        setRefreshing(true);
      } else {
        setLoading(true);
      }
      setError(null);
      const response = await GetSpotifyHomeFeed(forceRefresh);
      setFeed(response);
      setCategoryItems(null);
    } catch (err: unknown) {
      console.error('Failed to load Spotify home feed:', err);
      setError(
        err instanceof Error ? err.message : 'Failed to load discovery feed',
      );
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }, []);

  useEffect(() => {
    loadFeed(false);
  }, [loadFeed]);

  const handleCategoryClick = async (catId: string) => {
    setSelectedCategory(catId);
    if (
      catId === 'all' ||
      catId === 'new-releases' ||
      catId === 'trending' ||
      catId === 'playlists'
    ) {
      setCategoryItems(null);
      return;
    }

    try {
      setLoading(true);
      setError(null);
      const genre = CATEGORIES.find((c) => c.id === catId)?.label || catId;
      const items = await GetSpotifyCategoryFeed(genre);
      setCategoryItems(items);
    } catch (err: unknown) {
      console.error('Failed to load category feed:', err);
      setError(
        err instanceof Error ? err.message : 'Failed to load category items',
      );
    } finally {
      setLoading(false);
    }
  };

  const getItemIcon = (type: string) => {
    switch (type) {
      case 'album':
        return <Disc className='h-3 w-3' />;
      case 'playlist':
        return <Radio className='h-3 w-3' />;
      case 'artist':
        return <User className='h-3 w-3' />;
      default:
        return <Music2 className='h-3 w-3' />;
    }
  };

  const handleItemClick = (url: string) => {
    setSelectedItemUrl(url);
    onSelectItem(url);
  };

  const renderCard = (item: backend.SpotifyHomeItem, index: number) => {
    const isThisItemLoading =
      isLoadingItem && selectedItemUrl === item.external_urls;
    return (
      <Card
        key={`${item.id}-${index}`}
        onClick={() => handleItemClick(item.external_urls)}
        className={`group relative cursor-pointer overflow-hidden border border-border/60 bg-card/70 hover:bg-accent/40 hover:border-primary/40 hover:shadow-md transition-all duration-200 p-2.5 rounded-xl flex flex-col justify-between ${isThisItemLoading ? 'ring-2 ring-primary' : ''}`}
      >
        <div className='relative aspect-square w-full overflow-hidden rounded-lg bg-muted/60 mb-2.5'>
          {item.images ? (
            <img
              src={item.images}
              alt={item.name}
              loading='lazy'
              className='h-full w-full object-cover transition-transform duration-300 group-hover:scale-105'
            />
          ) : (
            <div className='flex h-full w-full items-center justify-center bg-muted'>
              <Disc className='h-8 w-8 text-muted-foreground/40' />
            </div>
          )}

          {/* Floating play / loading action overlay */}
          <div
            className={`absolute right-2 bottom-2 h-9 w-9 rounded-full bg-primary text-primary-foreground shadow-lg flex items-center justify-center transition-all duration-200 ${isThisItemLoading ? 'opacity-100 translate-y-0' : 'opacity-0 translate-y-2 group-hover:opacity-100 group-hover:translate-y-0'}`}
          >
            {isThisItemLoading ? (
              <Loader2 className='h-4 w-4 animate-spin' />
            ) : (
              <Play className='h-4 w-4 fill-current ml-0.5' />
            )}
          </div>

          {/* Type Badge */}
          <div className='absolute top-2 left-2'>
            <Badge
              variant='secondary'
              className='h-5 text-[10px] px-1.5 py-0 backdrop-blur-md bg-background/80 capitalize flex items-center gap-1 font-medium'
            >
              {getItemIcon(item.type)}
              {item.type}
            </Badge>
          </div>
        </div>

        <div className='flex flex-col gap-0.5 px-0.5'>
          <h3
            className='text-xs font-semibold leading-tight line-clamp-1 group-hover:text-primary transition-colors'
            title={item.name}
          >
            {item.name}
          </h3>
          <p
            className='text-[11px] text-muted-foreground line-clamp-1'
            title={item.subtitle}
          >
            {item.subtitle || 'Spotify'}
          </p>
          {item.release_date && (
            <p className='text-[10px] text-muted-foreground/70 mt-0.5'>
              {item.release_date}
            </p>
          )}
        </div>
      </Card>
    );
  };

  const renderQuickPick = (item: backend.SpotifyHomeItem, index: number) => {
    const isThisItemLoading =
      isLoadingItem && selectedItemUrl === item.external_urls;
    return (
      <div
        key={`quick-${item.id}-${index}`}
        onClick={() => handleItemClick(item.external_urls)}
        className={`group relative flex items-center gap-3 overflow-hidden rounded-lg border border-border/50 bg-card/60 hover:bg-accent/50 hover:border-primary/40 cursor-pointer p-1.5 pr-3 transition-all duration-150 ${isThisItemLoading ? 'ring-2 ring-primary' : ''}`}
      >
        <div className='h-12 w-12 shrink-0 overflow-hidden rounded-md bg-muted'>
          {item.images ? (
            <img
              src={item.images}
              alt={item.name}
              className='h-full w-full object-cover transition-transform duration-200 group-hover:scale-105'
            />
          ) : (
            <div className='flex h-full w-full items-center justify-center'>
              <Radio className='h-5 w-5 text-muted-foreground' />
            </div>
          )}
        </div>
        <div className='flex-1 min-w-0'>
          <p className='text-xs font-semibold leading-snug line-clamp-1 group-hover:text-primary transition-colors'>
            {item.name}
          </p>
          <p className='text-[10px] text-muted-foreground line-clamp-1'>
            {item.subtitle || 'Playlist'}
          </p>
        </div>
        <div
          className={`shrink-0 transition-opacity ${isThisItemLoading ? 'opacity-100' : 'opacity-0 group-hover:opacity-100'}`}
        >
          <div className='h-7 w-7 rounded-full bg-primary text-primary-foreground flex items-center justify-center shadow'>
            {isThisItemLoading ? (
              <Loader2 className='h-3.5 w-3.5 animate-spin' />
            ) : (
              <Play className='h-3.5 w-3.5 fill-current ml-0.5' />
            )}
          </div>
        </div>
      </div>
    );
  };

  const renderSkeletons = (count = 12) => {
    return (
      <div className='grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6 gap-3'>
        {Array.from({ length: count }).map((_, i) => (
          <div
            key={`skel-${i}`}
            className='p-2.5 rounded-xl border border-border/40 bg-card/40 animate-pulse space-y-2.5'
          >
            <div className='aspect-square w-full rounded-lg bg-muted/60' />
            <div className='h-3 w-4/5 rounded bg-muted/70' />
            <div className='h-2.5 w-3/5 rounded bg-muted/40' />
          </div>
        ))}
      </div>
    );
  };

  return (
    <div className='w-full space-y-6 animate-in fade-in duration-300'>
      {/* Header & Greeting Banner */}
      <div className='flex flex-col sm:flex-row sm:items-center justify-between gap-3 pb-1 border-b border-border/40'>
        <div className='flex items-center gap-2.5'>
          <div className='h-9 w-9 rounded-xl bg-primary/10 text-primary flex items-center justify-center shadow-inner'>
            <Sparkles className='h-5 w-5' />
          </div>
          <div>
            <h1 className='text-lg md:text-xl font-bold tracking-tight'>
              {feed?.greeting || 'Discover Music'}
            </h1>
            <p className='text-xs text-muted-foreground'>
              Browse fresh releases, trending albums, and Spotify curated charts
            </p>
          </div>
        </div>

        <div className='flex items-center gap-2'>
          <Button
            variant='outline'
            size='sm'
            onClick={() => loadFeed(true)}
            disabled={refreshing || loading}
            className='h-8 gap-1.5 text-xs text-muted-foreground hover:text-foreground'
          >
            <RefreshCw
              className={`h-3.5 w-3.5 ${refreshing ? 'animate-spin text-primary' : ''}`}
            />
            <span>Refresh</span>
          </Button>
        </div>
      </div>

      {/* Category / Genre Filters */}
      <div className='flex items-center gap-1.5 overflow-x-auto pb-1 no-scrollbar'>
        {CATEGORIES.map((cat) => {
          const isActive = selectedCategory === cat.id;
          return (
            <button
              key={cat.id}
              onClick={() => handleCategoryClick(cat.id)}
              className={`px-3 py-1.5 rounded-full text-xs font-medium whitespace-nowrap transition-all duration-150 ${
                isActive
                  ? 'bg-primary text-primary-foreground shadow-sm'
                  : 'bg-muted/60 hover:bg-muted text-muted-foreground hover:text-foreground'
              }`}
            >
              {cat.label}
            </button>
          );
        })}
      </div>

      {/* Error Message if any */}
      {error && (
        <div className='rounded-xl border border-destructive/30 bg-destructive/10 p-4 text-center space-y-2'>
          <p className='text-xs text-destructive font-medium'>{error}</p>
          <Button
            variant='outline'
            size='sm'
            onClick={() => loadFeed(true)}
            className='h-7 text-xs'
          >
            Try Again
          </Button>
        </div>
      )}

      {/* Loading Indicator */}
      {loading && renderSkeletons(12)}

      {/* Category Specific View (e.g. Pop, Rock, Electronic...) */}
      {!loading && categoryItems && (
        <div className='space-y-4'>
          <div className='flex items-center justify-between'>
            <div className='flex items-center gap-2'>
              <Flame className='h-4 w-4 text-primary' />
              <h2 className='text-sm font-semibold tracking-tight'>
                {CATEGORIES.find((c) => c.id === selectedCategory)?.label ||
                  'Genre'}{' '}
                Releases & Playlists
              </h2>
            </div>
            <span className='text-xs text-muted-foreground'>
              {categoryItems.length} items
            </span>
          </div>

          <div className='grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6 gap-3'>
            {categoryItems.map((item, idx) => renderCard(item, idx))}
          </div>
        </div>
      )}

      {/* Main Feed Sections */}
      {!loading && !categoryItems && feed && (
        <div className='space-y-8'>
          {/* Quick Picks Shelf */}
          {(selectedCategory === 'all' || selectedCategory === 'playlists') &&
            feed.quick_picks &&
            feed.quick_picks.length > 0 && (
              <div className='space-y-3'>
                <h2 className='text-xs font-semibold uppercase tracking-wider text-muted-foreground flex items-center gap-1.5'>
                  <Radio className='h-3.5 w-3.5 text-primary' />
                  Quick Hits & Top Charts
                </h2>
                <div className='grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 gap-2.5'>
                  {feed.quick_picks.map((item, idx) =>
                    renderQuickPick(item, idx),
                  )}
                </div>
              </div>
            )}

          {/* Filtered or All Sections */}
          {feed.sections
            .filter((section) => {
              if (selectedCategory === 'all') return true;
              if (selectedCategory === 'new-releases')
                return section.id === 'new-releases';
              if (selectedCategory === 'trending')
                return section.id === 'trending-albums';
              if (selectedCategory === 'playlists')
                return section.id === 'featured-playlists';
              return true;
            })
            .map((section) => (
              <div key={section.id} className='space-y-3'>
                <div className='flex items-center justify-between'>
                  <div>
                    <h2 className='text-sm font-semibold tracking-tight flex items-center gap-2'>
                      {section.id === 'new-releases' && (
                        <Disc className='h-4 w-4 text-primary' />
                      )}
                      {section.id === 'trending-albums' && (
                        <Flame className='h-4 w-4 text-primary' />
                      )}
                      {section.id === 'featured-playlists' && (
                        <Radio className='h-4 w-4 text-primary' />
                      )}
                      {section.id === 'popular-artists' && (
                        <User className='h-4 w-4 text-primary' />
                      )}
                      {section.title}
                    </h2>
                    {section.description && (
                      <p className='text-[11px] text-muted-foreground'>
                        {section.description}
                      </p>
                    )}
                  </div>
                  <span className='text-xs text-muted-foreground'>
                    {section.items.length} items
                  </span>
                </div>

                <div className='grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6 gap-3'>
                  {section.items.map((item, idx) => renderCard(item, idx))}
                </div>
              </div>
            ))}
        </div>
      )}
    </div>
  );
}

