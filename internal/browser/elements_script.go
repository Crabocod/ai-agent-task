package browser

func getElementsScript() string {
	return `(() => {
		try {
			const result = [];
			const seen = new Set();
			const all = document.querySelectorAll('*');
			
			const priorityTags = ['a', 'button', 'input', 'select', 'textarea'];
			const secondaryTags = ['h1', 'h2', 'h3', 'h4', 'h5', 'h6', 'span', 'div', 'label'];
			
			const generateSelector = (el) => {
				const tag = el.tagName.toLowerCase();
				const selectors = [];

				const qaAttrs = ['data-test-id', 'data-testid', 'data-test', 'data-qa', 'data-cy'];
				for (const attr of qaAttrs) {
					const val = el.getAttribute(attr);
					if (val) {
						selectors.push(tag + '[' + attr + '="' + val + '"]');
						break;
					}
				}

				if (el.id && /^[a-zA-Z]/.test(el.id) && !el.id.includes(' ')) {
					selectors.push('#' + el.id);
				}
				
				if (el.name && ['input', 'select', 'textarea', 'button'].includes(tag)) {
					selectors.push(tag + '[name="' + el.name + '"]');
				}
				
				const ariaLabel = el.getAttribute('aria-label');
				if (ariaLabel && ariaLabel.length < 80) {
					selectors.push('[aria-label="' + ariaLabel + '"]');
				}
				
				const role = el.getAttribute('role');
				if (role) {
					if (ariaLabel) {
						selectors.push('[role="' + role + '"][aria-label="' + ariaLabel + '"]');
					} else {
						selectors.push('[role="' + role + '"]');
					}
				}
				
				if (el.type && tag === 'input') {
					if (el.placeholder) {
						selectors.push('input[type="' + el.type + '"][placeholder="' + el.placeholder + '"]');
					} else {
						selectors.push('input[type="' + el.type + '"]');
					}
				}
				
				if (el.className && typeof el.className === 'string') {
					const classes = el.className.split(' ')
						.filter(c => c && !c.match(/^[0-9]/) && c.length < 40 && !c.match(/^[a-f0-9]{8,}$/))
						.slice(0, 2);
					if (classes.length > 0) {
						selectors.push('.' + classes.join('.'));
					}
				}
				
				const title = el.getAttribute('title');
				if (title && title.length < 50) {
					selectors.push('[title="' + title + '"]');
				}
				
				if (selectors.length === 0) {
					let path = [];
					let current = el;
					let depth = 0;
					
					while (current && current.tagName && depth < 3) {
						const t = current.tagName.toLowerCase();
						if (current.id) {
							path.unshift('#' + current.id);
							break;
						}
						const index = Array.from(current.parentNode?.children || []).indexOf(current);
						if (index >= 0) {
							path.unshift(t + ':nth-child(' + (index + 1) + ')');
						}
						current = current.parentElement;
						depth++;
					}
					
					if (path.length > 0) {
						selectors.push(path.join(' > '));
					} else {
						selectors.push(tag);
					}
				}
				
				return selectors[0];
			};
			
			const hasIconChild = (el) => {
				const svg = el.querySelector('svg');
				if (svg) {
					const svgText = svg.outerHTML.toLowerCase();
					if (svgText.includes('plus') || svgText.includes('+') || svgText.includes('add')) {
						return true;
					}
					const paths = svg.querySelectorAll('path, circle, line');
					if (paths.length > 0 && paths.length < 5) {
						return true;
					}
				}
				
				const icon = el.querySelector('[class*="icon"], [class*="Icon"], i');
				if (icon) return true;
				
				const rect = el.getBoundingClientRect();
				if (rect.width < 60 && rect.height < 60 && rect.width > 20 && rect.height > 20) {
					const text = (el.innerText || el.textContent || '').trim();
					if (text.length === 0 || text === '+' || text === '-') {
						return true;
					}
				}
				
				return false;
			};
			
			const processByPriority = (tags, maxPerTag) => {
				tags.forEach(targetTag => {
					let count = 0;
					for (let i = 0; i < all.length && count < maxPerTag; i++) {
						const el = all[i];
						const tag = el.tagName.toLowerCase();
						
						if (tag !== targetTag) continue;
						if (seen.has(el)) continue;
						
						const rect = el.getBoundingClientRect();
						const style = window.getComputedStyle(el);
						
						const isVisible = (
							rect.width > 0 && 
							rect.height > 0 && 
							style.display !== 'none' && 
							style.visibility !== 'hidden' &&
							style.opacity !== '0' &&
							rect.top < window.innerHeight + 500 &&
							rect.bottom > -500
						);
						
						if (!isVisible) continue;
						
						seen.add(el);
						count++;
						
						const ariaLabel = el.getAttribute('aria-label');
						const testId = el.getAttribute('data-test-id') || el.getAttribute('data-testid');
						const role = el.getAttribute('role');
						
						let txt = '';
						if (el.value) {
						 txt = el.value;
						} else if (el.innerText && el.innerText.trim()) {
						 txt = el.innerText;
						} else if (el.textContent && el.textContent.trim()) {
						 txt = el.textContent;
						} else if (ariaLabel) {
						txt = ariaLabel;
						} else if (testId) {
						 const parts = testId.split(/[-_.]/g);
						 if (parts.length > 0) {
						 txt = '[' + parts.filter(p => p.length > 2).slice(0, 3).join(' ').toUpperCase() + ']';
						}
						}
						
					txt = txt.trim();
						if (txt.length > 200) {
							txt = txt.substring(0, 200) + '...';
						}
						
						const sel = generateSelector(el);
						
						const attrs = {};
						if (el.type) attrs.type = el.type;
						if (el.placeholder) attrs.placeholder = el.placeholder.substring(0, 50);
						if (el.name) attrs.name = el.name;
						if (ariaLabel) attrs['aria-label'] = ariaLabel.substring(0, 100);
						if (el.href) attrs.href = el.href.substring(0, 100);
						if (role) attrs.role = role;
						if (testId) attrs['data-test-id'] = testId;

						let isClickable = (
						['a', 'button', 'input', 'select'].includes(tag) ||
						el.onclick !== null ||
						role === 'button' ||
						role === 'link' ||
						role === 'tab' ||
						role === 'menuitem' ||
						style.cursor === 'pointer' ||
						(testId && (testId.includes('button') || testId.includes('add') || testId.includes('Button') || testId.includes('Add'))) ||
						 (ariaLabel && (ariaLabel.toLowerCase().includes('add') || ariaLabel.toLowerCase().includes('plus')))
					);

						if (tag === 'button' || tag === 'div') {
						if (hasIconChild(el)) {
						isClickable = true;
						if (!txt || txt.length === 0) {
						 txt = '[ICON_BUTTON]';
						 }
						 }
						
						if ((role === 'button' || el.onclick) && rect.width < 80 && rect.height < 80) {
							isClickable = true;
							if (!txt || txt.length < 2) {
								txt = '[SMALL_BUTTON]';
							}
						}
					}

						if (!isClickable && el.parentElement) {
							const parent = el.parentElement;
							const parentRole = parent.getAttribute('role');
							const parentTag = parent.tagName.toLowerCase();
							isClickable = (
								['a', 'button'].includes(parentTag) ||
								parentRole === 'button' ||
								parent.onclick !== null
							);
						}
						
						const centerX = Math.round(rect.left + rect.width / 2);
						const centerY = Math.round(rect.top + rect.height / 2);
						
						result.push({
							tag: tag,
							text: txt,
							selector: sel,
							attributes: attrs,
							visible: true,
							clickable: isClickable,
							x: centerX,
							y: centerY,
							width: Math.round(rect.width),
							height: Math.round(rect.height)
						});
					}
				});
			};
			
			processByPriority(priorityTags, 50);
			processByPriority(secondaryTags, 20);
			
			return result;
		} catch(e) {
			console.error('Error in GetElements:', e);
			return [];
		}
	})()`
}
